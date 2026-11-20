package main

import (
	"bufio"
	"context"
	"database/sql"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"strings"

	"github.com/DR-lin-eng/sub2api-no2api/internal/infrastructure/repository"
	"github.com/DR-lin-eng/sub2api-no2api/internal/modules/sitemigration"
	"github.com/DR-lin-eng/sub2api-no2api/internal/platform/config"
	"golang.org/x/term"
)

func migrate(args []string) error {
	if err := migrationImageUser(); err != nil {
		return err
	}
	if len(args) == 0 || (args[0] != "push" && args[0] != "pull") {
		return errors.New("usage: sub2api migrate push|pull --peer IP-or-domain [--scope all|clone|login|login-quota] [--resume UUID] [--report path]")
	}
	fs := flag.NewFlagSet("migrate "+args[0], flag.ContinueOnError)
	opts := sitemigration.Options{Direction: args[0], Output: os.Stderr}
	fs.StringVar(&opts.Peer, "peer", "", "remote HTTPS IP or domain")
	fs.StringVar(&opts.Profile, "scope", "all", "all, clone, login or login-quota")
	fs.StringVar(&opts.Resume, "resume", "", "resume the printed migration UUID")
	fs.StringVar(&opts.Report, "report", "", "new local CSV file")
	if err := fs.Parse(args[1:]); err != nil {
		return err
	}
	if fs.NArg() != 0 || opts.Peer == "" {
		return errors.New("--peer is required; enter the admin key on stdin")
	}
	if term.IsTerminal(int(os.Stdin.Fd())) {
		fmt.Fprint(os.Stderr, "对方站点的迁移管理员 Key（输入隐藏）：")
		b, e := term.ReadPassword(int(os.Stdin.Fd()))
		fmt.Fprintln(os.Stderr)
		if e != nil {
			return e
		}
		opts.Key = strings.TrimSpace(string(b))
	} else {
		input := bufio.NewReader(io.LimitReader(os.Stdin, 257))
		line, err := input.ReadString('\n')
		if err != nil && err != io.EOF {
			return err
		}
		opts.Key = strings.TrimSpace(line)
	}
	if len(opts.Key) < 16 || len(opts.Key) > 256 {
		return errors.New("invalid administrator key")
	}
	cfg, err := config.LoadForBootstrap()
	if err != nil {
		return err
	}
	db, err := sql.Open("postgres", cfg.Database.DSN())
	if err != nil {
		return errors.New("could not open local database")
	}
	defer func() { _ = db.Close() }()
	db.SetMaxOpenConns(4)
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt)
	defer cancel()
	return repository.ProvideSiteMigration(db, cfg).Transfer(ctx, opts)
}
