package sitemigration

import (
	"context"
	"database/sql"
	"errors"
	"net"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
)

// Panel starts transfers independently of the browser connection. The peer key
// stays with the worker. A restarted process needs a fresh operator-supplied key.
type Panel struct {
	store        Store
	privatePeers map[string]bool
	ctx          context.Context
	cancel       context.CancelFunc
	mu           sync.Mutex
	workers      sync.WaitGroup
	stopped      bool
}

func NewPanel(store Store, privatePeers []string) *Panel {
	ctx, cancel := context.WithCancel(context.Background())
	p := &Panel{store: store, privatePeers: map[string]bool{}, ctx: ctx, cancel: cancel}
	for _, s := range privatePeers {
		if o, e := canonicalPeer(s); e == nil {
			p.privatePeers[o] = true
		}
	}
	return p
}
func (p *Panel) Stop() { p.mu.Lock(); p.stopped = true; p.cancel(); p.mu.Unlock(); p.workers.Wait() }

type PanelJob struct {
	ID           string    `json:"id"`
	Direction    string    `json:"direction"`
	Profile      string    `json:"profile"`
	Peer         string    `json:"peer"`
	Phase        string    `json:"phase"`
	ErrorCode    string    `json:"error_code,omitempty"`
	Acknowledged int       `json:"acknowledged"`
	Total        int       `json:"total"`
	Inserted     int64     `json:"inserted"`
	Issues       int       `json:"issues"`
	ReportBytes  int64     `json:"report_bytes"`
	CreatedAt    time.Time `json:"created_at"`
	ExpiresAt    time.Time `json:"expires_at"`
}

const jobColumns = `id,direction,profile,peer,CASE WHEN attempt IS NOT NULL AND lease_until<=now() THEN 'interrupted' ELSE phase END,error_code,acknowledged,total,inserted,issues,report_bytes,created_at,expires_at`

func scanPanelJob(src interface{ Scan(...any) error }) (PanelJob, error) {
	var j PanelJob
	err := src.Scan(&j.ID, &j.Direction, &j.Profile, &j.Peer, &j.Phase, &j.ErrorCode, &j.Acknowledged, &j.Total, &j.Inserted, &j.Issues, &j.ReportBytes, &j.CreatedAt, &j.ExpiresAt)
	if errors.Is(err, sql.ErrNoRows) {
		err = ErrMissing
	}
	return j, err
}
func (p *Panel) List(ctx context.Context, admin int64) ([]PanelJob, error) {
	// Expired CSV parts are deleted by cascade alongside the operator metadata.
	if _, err := p.store.DB.ExecContext(ctx, "DELETE FROM site_migration_panel_jobs WHERE expires_at<=now() AND (lease_until IS NULL OR lease_until<=now())"); err != nil {
		return nil, err
	}
	rows, err := p.store.DB.QueryContext(ctx, "SELECT "+jobColumns+" FROM site_migration_panel_jobs WHERE admin_id=$1 AND expires_at>now() ORDER BY created_at DESC LIMIT 100", admin)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	jobs := []PanelJob{}
	for rows.Next() {
		j, e := scanPanelJob(rows)
		if e != nil {
			return nil, e
		}
		jobs = append(jobs, j)
	}
	return jobs, rows.Err()
}

type PanelInput struct {
	ID        string `json:"id"`
	Direction string `json:"direction"`
	Profile   string `json:"profile"`
	Peer      string `json:"peer"`
	Key       string `json:"key"`
}
type panelTransfer struct {
	panel   *Panel
	admin   int64
	job     PanelJob
	attempt string
}

func (p *Panel) Start(ctx context.Context, admin int64, in PanelInput, resume bool) (PanelJob, error) {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	if !validID(in.ID) || admin <= 0 || len(in.Key) < 16 || len(in.Key) > 256 || strings.ContainsAny(in.Key, " \r\n\t") {
		return PanelJob{}, ErrInvalid
	}
	if !resume {
		var err error
		in.Peer, err = p.peerOrigin(in.Peer)
		if err != nil {
			return PanelJob{}, err
		}
		if !validProfile(in.Profile) || (in.Direction != "push" && in.Direction != "pull") {
			return PanelJob{}, ErrInvalid
		}
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.stopped {
		return PanelJob{}, ErrConflict
	}
	tx, err := p.store.DB.BeginTx(ctx, nil)
	if err != nil {
		return PanelJob{}, err
	}
	defer func() { _ = tx.Rollback() }()
	if _, err = tx.ExecContext(ctx, "SELECT pg_advisory_xact_lock(162884213,3)"); err != nil {
		return PanelJob{}, err
	}
	job, err := scanPanelJob(tx.QueryRowContext(ctx, "SELECT "+jobColumns+" FROM site_migration_panel_jobs WHERE id=$1 AND admin_id=$2 AND expires_at>now() FOR UPDATE", in.ID, admin))
	found := err == nil
	if !found && (!errors.Is(err, ErrMissing) || resume) {
		return PanelJob{}, err
	}
	if found && !resume {
		if job.Peer != in.Peer || job.Profile != in.Profile || job.Direction != in.Direction {
			return PanelJob{}, ErrConflict
		}
		return job, nil // Lost acknowledgement for the same start request.
	}
	if found && job.Phase != "failed" && job.Phase != "interrupted" {
		return job, nil
	}
	var live, retained int
	if err = tx.QueryRowContext(ctx, "SELECT count(*) FILTER(WHERE lease_until>now()),count(*) FROM site_migration_panel_jobs WHERE expires_at>now()").Scan(&live, &retained); err != nil {
		return PanelJob{}, err
	}
	if live >= maxLiveJobs || (!found && retained >= 100) {
		return PanelJob{}, ErrLimit
	}
	attempt := uuid.NewString()
	if found {
		if _, err = tx.ExecContext(ctx, "DELETE FROM site_migration_panel_reports WHERE job=$1", in.ID); err != nil {
			return PanelJob{}, err
		}
		job, err = scanPanelJob(tx.QueryRowContext(ctx, "UPDATE site_migration_panel_jobs SET attempt=$2,lease_until=now()+interval '90 seconds',phase='connecting',error_code='' WHERE id=$1 RETURNING "+jobColumns, in.ID, attempt))
	} else {
		job, err = scanPanelJob(tx.QueryRowContext(ctx, "INSERT INTO site_migration_panel_jobs(id,admin_id,direction,profile,peer,phase,attempt,lease_until) VALUES($1,$2,$3,$4,$5,'connecting',$6,now()+interval '90 seconds') ON CONFLICT DO NOTHING RETURNING "+jobColumns, in.ID, admin, in.Direction, in.Profile, in.Peer, attempt))
	}
	if err != nil {
		return PanelJob{}, err
	}
	if err = tx.Commit(); err != nil {
		return PanelJob{}, err
	}
	worker := &panelTransfer{p, admin, job, attempt}
	p.workers.Add(1)
	go func() { defer p.workers.Done(); worker.run(in.Key) }()
	return job, nil
}
func (w *panelTransfer) update(ctx context.Context, statement string, args ...any) error {
	r, e := w.panel.store.DB.ExecContext(ctx, statement, append([]any{w.job.ID, w.attempt}, args...)...)
	if e != nil {
		return e
	}
	n, e := r.RowsAffected()
	if e == nil && n != 1 {
		return ErrConflict
	}
	return e
}
func (w *panelTransfer) progress(ctx context.Context, phase string, done, total int) error {
	if w == nil {
		return nil
	}
	return w.update(ctx, "UPDATE site_migration_panel_jobs SET phase=$3,acknowledged=GREATEST(acknowledged,$4),total=GREATEST(total,$5) WHERE id=$1 AND attempt=$2", phase, done, total)
}
func (w *panelTransfer) run(key string) {
	ctx, cancel := context.WithDeadline(w.panel.ctx, w.job.ExpiresAt)
	stopped := make(chan struct{})
	go func() {
		defer close(stopped)
		timer := time.NewTicker(20 * time.Second)
		defer timer.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-timer.C:
				c, stop := context.WithTimeout(ctx, 10*time.Second)
				e := w.update(c, "UPDATE site_migration_panel_jobs SET lease_until=CASE WHEN phase='completed' THEN NULL ELSE now()+interval '90 seconds' END WHERE id=$1 AND attempt=$2")
				stop()
				if e != nil {
					cancel()
					return
				}
			}
		}
	}()
	err := w.panel.store.Transfer(ctx, Options{Direction: w.job.Direction, Peer: w.job.Peer, Profile: w.job.Profile, Key: key, panel: w})
	cancel()
	<-stopped
	if err != nil {
		c, stop := context.WithTimeout(context.Background(), 5*time.Second)
		defer stop()
		_ = w.update(c, "UPDATE site_migration_panel_jobs SET phase='failed',error_code=$3,attempt=NULL,lease_until=NULL WHERE id=$1 AND attempt=$2 AND phase<>'completed'", panelError(err))
	}
}
func panelError(e error) string {
	var peer *peerResponseError
	switch {
	case errors.As(e, &peer):
		switch peer.status {
		case 412:
			return "clone_policy"
		case 424:
			return "billing_pending"
		case 401, 403:
			return "peer_auth"
		case 404:
			return "peer_missing"
		case 423:
			return "peer_locked"
		case 409:
			return "conflict"
		case 413, 429:
			return "limit"
		default:
			return "peer_error"
		}
	case errors.Is(e, ErrClonePolicy):
		return "clone_policy"
	case errors.Is(e, ErrBillingPending):
		return "billing_pending"
	case errors.Is(e, ErrPeerAddress):
		return "address"
	case errors.Is(e, ErrMissing):
		return "checkpoint"
	case errors.Is(e, ErrConflict):
		return "conflict"
	case errors.Is(e, ErrLimit):
		return "limit"
	case errors.Is(e, context.Canceled), errors.Is(e, context.DeadlineExceeded):
		return "interrupted"
	}
	var netErr net.Error
	if errors.As(e, &netErr) {
		return "connection"
	}
	return "transfer"
}
