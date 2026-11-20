package sitemigration

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/csv"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
)

type client struct {
	base, key, id, mode, profile string
	http                         *http.Client
}

func peerOrigin(raw string) (string, error) {
	if !strings.Contains(raw, "://") {
		raw = "https://" + raw
	}
	u, err := url.Parse(raw)
	if err != nil || u.Scheme != "https" || u.Hostname() == "" || u.User != nil || u.RawQuery != "" || u.ForceQuery || u.Fragment != "" || u.RawFragment != "" || (u.Path != "" && u.Path != "/") || u.Opaque != "" {
		return "", errors.New("peer must be an HTTPS IP or domain, optionally with a port")
	}
	return "https://" + u.Host, nil
}
func (c *client) request(ctx context.Context, action string, n int, payload []byte) ([]byte, error) {
	u := c.base + "/api/v1/admin/site-migration/" + c.mode + "/" + action + "?id=" + c.id + "&n=" + strconv.Itoa(n)
	if action == "probe" {
		u += "&scope=" + url.QueryEscape(c.profile)
	}
	var last error
	var delay time.Duration
	for try := 0; try < 5; try++ {
		if try > 0 {
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(delay):
			}
		}
		delay = time.Second << uint(try)
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, u, bytes.NewReader(payload))
		if err != nil {
			return nil, err
		}
		req.Header.Set("x-api-key", c.key)
		req.Header.Set("Content-Type", "application/octet-stream")
		res, err := c.http.Do(req)
		if err != nil {
			last = err
			continue
		}
		b, e := io.ReadAll(io.LimitReader(res.Body, BlockSize+65))
		res.Body.Close()
		if e != nil {
			last = e
			continue
		}
		if res.StatusCode != 200 {
			if action == "delete" && res.StatusCode == http.StatusNotFound {
				return nil, nil
			}
			last = fmt.Errorf("peer rejected %s (HTTP %d)", action, res.StatusCode)
			if seconds, e := strconv.Atoi(res.Header.Get("Retry-After")); e == nil && seconds > 0 {
				if seconds > 120 {
					return nil, last
				}
				delay = max(delay, time.Duration(seconds)*time.Second)
			}
			if res.StatusCode == 429 || res.StatusCode == 502 || res.StatusCode == 503 || res.StatusCode == 504 {
				continue
			}
			return nil, last
		}
		if len(b) > BlockSize+64 {
			return nil, ErrLimit
		}
		return b, nil
	}
	return nil, last
}
func (c *client) state(ctx context.Context, op string, n int, p []byte) (State, error) {
	b, e := c.request(ctx, op, n, p)
	var st State
	if e != nil {
		return st, e
	}
	if json.Unmarshal(b, &st) != nil || st.ID != c.id || st.Mode != c.mode || !validProfile(st.Profile) || len(st.Public) != 32 || st.Blocks < 0 || st.Blocks > maxBlocks || st.Errors < 0 || st.Errors > 2_000_000 {
		return st, errors.New("invalid peer status")
	}
	return st, nil
}

type Options struct {
	Direction, Peer, Profile, Key, Resume, Report string
	Output                                        io.Writer
}

func (s Store) Transfer(ctx context.Context, o Options) error {
	base, err := peerOrigin(o.Peer)
	if err != nil {
		return err
	}
	if !validProfile(o.Profile) || o.Key == "" || (o.Direction != "push" && o.Direction != "pull") {
		return ErrInvalid
	}
	mode, remoteMode := "export", "import"
	if o.Direction == "pull" {
		mode, remoteMode = "import", "export"
	}
	id := o.Resume
	if id == "" {
		id = uuid.NewString()
	}
	if !validID(id) {
		return ErrInvalid
	}
	a := Auth{Owner: "cli/" + base, Key: o.Key}
	transport := &http.Transport{TLSClientConfig: &tls.Config{MinVersion: tls.VersionTLS13}, TLSHandshakeTimeout: 15 * time.Second, IdleConnTimeout: time.Minute, DisableCompression: true}
	defer transport.CloseIdleConnections()
	c := &client{base: base, key: o.Key, id: id, mode: remoteMode, profile: o.Profile, http: &http.Client{Transport: transport, Timeout: 31 * time.Minute, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}}
	if o.Output == nil {
		o.Output = io.Discard
	}
	fmt.Fprintf(o.Output, "任务 %s。中断后使用相同参数并加上 --resume %s。\n", id, id)
	probe, err := c.request(ctx, "probe", 0, nil)
	if err != nil {
		return err
	}
	var exists bool
	if err = json.Unmarshal(probe, &exists); err != nil {
		return ErrInvalid
	}
	if remoteMode == "export" {
		exists, err = s.Existing(ctx)
		if err != nil {
			return err
		}
	}
	if o.Profile == "clone" {
		fmt.Fprintf(o.Output, "将在接收站创建独立数据库 %s。接收站已有的业务继续使用原库，切换由管理员执行。\n", s.cloneName(id))
	} else if exists {
		fmt.Fprintln(o.Output, "接收站已有数据。将只导入无冲突的记录，同邮箱及其他跳过原因会写入 CSV。")
	}
	if o.Resume != "" {
		if _, err = s.Status(ctx, a, id, mode); err != nil {
			return err
		}
	}
	local, err := s.Open(ctx, a, id, mode, o.Profile, nil)
	if err != nil {
		return err
	}
	payload, _ := json.Marshal(struct {
		Profile string
		Public  []byte
	}{o.Profile, local.Public})
	remote, err := c.state(ctx, "open", 0, payload)
	if err != nil {
		return err
	}
	if remote.Profile != o.Profile {
		return ErrConflict
	}
	local, err = s.Bind(ctx, a, id, mode, remote.Public)
	if err != nil {
		return err
	}
	src, dst := local, remote
	if mode == "export" {
		src, err = s.Prepare(ctx, a, id)
	} else {
		src, err = c.state(ctx, "prepare", 0, nil)
		dst = local
	}
	if err != nil {
		return err
	}
	if !src.Sealed || src.Blocks == 0 || dst.Blocks > src.Blocks {
		return ErrConflict
	}
	fmt.Fprintf(o.Output, "快照共 %d 块，接收站已确认 %d 块。\n", src.Blocks, dst.Blocks)
	for n := dst.Blocks; n < src.Blocks; n++ {
		var b []byte
		if mode == "export" {
			b, err = s.Block(ctx, a, id, mode, 0, n)
			if err == nil {
				dst, err = c.state(ctx, "block", n, b)
			}
		} else {
			b, err = c.request(ctx, "block", n, nil)
			if err == nil {
				dst, err = s.Receive(ctx, a, id, n, b)
			}
		}
		if err != nil {
			return err
		}
		if dst.Blocks != n+1 {
			return ErrConflict
		}
		if (n+1)%64 == 0 {
			fmt.Fprintf(o.Output, "已确认 %d / %d 块。\n", n+1, src.Blocks)
		}
	}
	if mode == "export" {
		dst, err = c.state(ctx, "apply", 0, nil)
	} else {
		dst, err = s.Apply(ctx, a, id)
	}
	if err != nil {
		return err
	}
	if !dst.Applied {
		return ErrConflict
	}
	path := o.Report
	if path == "" {
		path = filepath.Join(s.DataDir, "migration-errors-"+id+".csv")
	}
	err = writeReport(path, func(w io.Writer) error {
		if _, e := io.WriteString(w, "\uFEFF数据表,源记录ID,邮箱,跳过原因\r\n"); e != nil {
			return e
		}
		csvOut := csv.NewWriter(w)
		csvOut.UseCRLF = true
		var reportBytes int64
		for first := 0; first < dst.Errors; {
			var page []byte
			var e error
			if mode == "export" {
				page, e = c.request(ctx, "report", first, nil)
			} else {
				page, e = s.ReportPage(ctx, a, id, first)
			}
			if e != nil {
				return e
			}
			count, e := s.DecodeReportPage(ctx, a, id, mode, first, dst.Errors-first, page, func(line []byte) error {
				reportBytes += int64(len(line))
				if reportBytes > 1<<30 {
					return ErrLimit
				}
				return writeReportRecord(csvOut, line)
			})
			if e != nil {
				return e
			}
			first += count
		}
		csvOut.Flush()
		return csvOut.Error()
	})
	if err != nil {
		return err
	}
	fmt.Fprintf(o.Output, "已导入 %d 条，冲突或需处理 %d 条。冲突报告 %s\n", dst.Inserted, dst.Errors, path)
	if o.Profile == "clone" {
		fmt.Fprintf(o.Output, "接收站数据库 %s，页面目录为接收站 pricing.data_dir 下的 %s。切换时停写源站、核对快照后的变动，并把所有接收节点指向克隆库、独立 Redis 及对应页面目录，再重启验证。\n", s.cloneName(id), clonePageDirectory("", id))
	}
	// Receipt and checkpoint remain recoverable until the local report exists.
	if _, err = c.request(ctx, "delete", 0, nil); err != nil {
		fmt.Fprintln(o.Output, "远端检查点清理未确认，任务已完成。")
	}
	if _, err = s.Remove(ctx, a, id, mode); err != nil {
		fmt.Fprintln(o.Output, "本地检查点清理未确认，任务已完成。")
	}
	return nil
}
func writeReportRecord(w *csv.Writer, data []byte) error {
	r := csv.NewReader(bytes.NewReader(data))
	r.FieldsPerRecord = 4
	cells, err := r.Read()
	if err != nil {
		return ErrInvalid
	}
	if _, err = r.Read(); err != io.EOF {
		return ErrInvalid
	}
	for i, v := range cells {
		if strings.IndexByte(v, 0) >= 0 {
			return ErrInvalid
		}
		cells[i] = safeCell(v)
	}
	return w.Write(cells)
}

func writeReport(path string, write func(io.Writer) error) error {
	f, err := os.CreateTemp(filepath.Dir(path), ".migration-report-")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	err = write(f)
	if err == nil {
		err = f.Sync()
	}
	ce := f.Close()
	if err != nil {
		return err
	}
	if ce != nil {
		return ce
	}
	// No clobber, including symlinks and existing operator reports.
	if err = os.Link(f.Name(), path); err != nil {
		return err
	}
	dir, err := os.Open(filepath.Dir(path))
	if err != nil {
		return err
	}
	defer dir.Close()
	return dir.Sync()
}
