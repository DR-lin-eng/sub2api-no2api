package sitemigration

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path"
	"strings"

	"github.com/google/uuid"
)

type pagePart struct {
	Name      string
	Offset    int64
	Data      []byte
	End       bool
	Directory bool `json:",omitempty"`
}

const pageChunkSize = 192 << 10
const receiptFile = "pages.sha256"

func safePageName(n string) bool {
	return n != "." && len(n) <= 512 && fs.ValidPath(n) && !strings.ContainsAny(n, "\\:\x00\r\n") && !strings.HasPrefix(n, ".") && !strings.Contains(n, "/.")
}
func syncPageDir(root *os.Root, name string) error {
	f, err := root.Open(name)
	if err != nil {
		return err
	}
	defer f.Close()
	return f.Sync()
}

// Require a real directory at each provisioned boundary, not a symlink inside
// or outside the root. Check that the opened handle is the inspected inode.
func pageSubdir(parent *os.Root, name string, create bool) (*os.Root, error) {
	if create {
		err := parent.Mkdir(name, 0700)
		if err != nil && !errors.Is(err, os.ErrExist) {
			return nil, err
		}
		if err == nil {
			if err = syncPageDir(parent, "."); err != nil {
				return nil, err
			}
		}
	}
	info, err := parent.Lstat(name)
	if err != nil {
		return nil, err
	}
	if !info.IsDir() {
		return nil, errors.New("page path must be a real directory")
	}
	child, err := parent.OpenRoot(name)
	if err != nil {
		return nil, err
	}
	opened, err := child.Stat(".")
	if err != nil || !os.SameFile(info, opened) {
		child.Close()
		return nil, errors.New("page directory changed")
	}
	return child, nil
}
func pageFile(root *os.Root, name string) (*os.File, error) {
	info, err := root.Lstat(name)
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() {
		return nil, errors.New("page path must be a regular file")
	}
	f, err := root.Open(name)
	if err != nil {
		return nil, err
	}
	opened, err := f.Stat()
	if err != nil || !os.SameFile(info, opened) {
		f.Close()
		return nil, errors.New("page file changed")
	}
	return f, nil
}
func exportPages(dataDir string, enc *json.Encoder) error {
	base, err := os.OpenRoot(dataDir)
	if err != nil {
		return err
	}
	defer base.Close()
	root, err := pageSubdir(base, "pages", false)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	defer root.Close()
	buf := make([]byte, pageChunkSize)
	return fs.WalkDir(root.FS(), ".", func(n string, d fs.DirEntry, e error) error {
		if e != nil {
			return e
		}
		if n == "." {
			return nil
		}
		if !safePageName(n) || d.Type()&os.ModeSymlink != 0 {
			return ErrInvalid
		}
		if d.IsDir() {
			return enc.Encode(cloneFrame{File: &pagePart{Name: n, Directory: true}})
		}
		if !d.Type().IsRegular() {
			return ErrInvalid
		}
		f, e := pageFile(root, n)
		if e != nil {
			return e
		}
		defer f.Close()
		var off int64
		for {
			got, e := io.ReadFull(f, buf)
			end := e == io.EOF || e == io.ErrUnexpectedEOF
			if e != nil && !end {
				return e
			}
			if e = enc.Encode(cloneFrame{File: &pagePart{Name: n, Offset: off, Data: buf[:got], End: end}}); e != nil {
				return e
			}
			off += int64(got)
			if end {
				return nil
			}
		}
	})
}

type pageRestore struct {
	job, root *os.Root
	id, stage string
	f         *os.File
	name      string
	offset    int64
}

func pageJobRoot(data, id string, create bool) (*os.Root, error) {
	if !validID(id) {
		return nil, ErrInvalid
	}
	base, err := os.OpenRoot(data)
	if err != nil {
		return nil, err
	}
	defer base.Close()
	clones, err := pageSubdir(base, "site-migration-clones", create)
	if err != nil {
		return nil, err
	}
	defer clones.Close()
	return pageSubdir(clones, id, create)
}
func newPageRestore(data, id string) (*pageRestore, error) {
	job, err := pageJobRoot(data, id, true)
	if err != nil {
		return nil, err
	}
	stage := ".pages-" + uuid.NewString()
	if err = job.Mkdir(stage, 0700); err != nil {
		job.Close()
		return nil, err
	}
	root, err := pageSubdir(job, stage, false)
	if err != nil {
		job.RemoveAll(stage)
		job.Close()
		return nil, err
	}
	return &pageRestore{job: job, root: root, id: id, stage: stage}, nil
}
func (p *pageRestore) close() {
	if p.f != nil {
		p.f.Close()
	}
	p.root.Close()
	if p.stage != "" {
		_ = p.job.RemoveAll(p.stage)
	}
	p.job.Close()
}
func (p *pageRestore) write(part pagePart) error {
	if !safePageName(part.Name) {
		return ErrInvalid
	}
	if part.Directory {
		if p.f != nil || part.End || part.Offset != 0 || len(part.Data) != 0 {
			return ErrInvalid
		}
		return p.root.MkdirAll(part.Name, 0700)
	}
	if len(part.Data) > pageChunkSize || part.Offset < 0 || (!part.End && len(part.Data) == 0) {
		return ErrInvalid
	}
	if p.f == nil {
		if part.Offset != 0 {
			return ErrInvalid
		}
		if err := p.root.MkdirAll(path.Dir(part.Name), 0700); err != nil {
			return err
		}
		f, err := p.root.OpenFile(part.Name, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
		if err != nil {
			return err
		}
		p.f, p.name, p.offset = f, part.Name, 0
	}
	if part.Name != p.name || part.Offset != p.offset {
		return ErrInvalid
	}
	if _, err := p.f.Write(part.Data); err != nil {
		return err
	}
	p.offset += int64(len(part.Data))
	if part.End {
		if err := p.f.Sync(); err != nil {
			return err
		}
		if err := p.f.Close(); err != nil {
			return err
		}
		p.f = nil
	}
	return nil
}

// WalkDir's lexical order plus a typed, length-prefixed relative name and a
// fixed-width content digest commits to file bytes, names and empty folders.
func pageTreeDigest(root *os.Root, id string, syncDirs bool) (string, error) {
	sum := sha256.New()
	fmt.Fprintf(sum, "sub2api clone pages v1/%s\n", id)
	err := fs.WalkDir(root.FS(), ".", func(name string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if name != "." && !safePageName(name) {
			return ErrInvalid
		}
		if d.IsDir() {
			fmt.Fprintf(sum, "D%d:%s", len(name), name)
			if syncDirs {
				return syncPageDir(root, name)
			}
			return nil
		}
		if !d.Type().IsRegular() {
			return ErrInvalid
		}
		f, e := pageFile(root, name)
		if e != nil {
			return e
		}
		defer f.Close()
		h := sha256.New()
		if _, e = io.Copy(h, f); e != nil {
			return e
		}
		fmt.Fprintf(sum, "F%d:%s", len(name), name)
		sum.Write(h.Sum(nil))
		return nil
	})
	return hex.EncodeToString(sum.Sum(nil)), err
}
func readPageReceipt(job *os.Root) (string, error) {
	f, err := pageFile(job, receiptFile)
	if err != nil {
		return "", err
	}
	defer f.Close()
	b, err := io.ReadAll(io.LimitReader(f, sha256.Size*2+1))
	if err != nil {
		return "", err
	}
	if len(b) != sha256.Size*2 {
		return "", ErrInvalid
	}
	if _, e := hex.DecodeString(string(b)); e != nil {
		return "", ErrInvalid
	}
	return string(b), nil
}
func (p *pageRestore) writeReceipt(digest string) error {
	// Publish the complete receipt without ever overwriting an existing one.
	tmp := ".receipt-" + uuid.NewString()
	f, err := p.job.OpenFile(tmp, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		return err
	}
	defer p.job.Remove(tmp)
	if _, err = f.WriteString(digest); err == nil {
		err = f.Sync()
	}
	ce := f.Close()
	if err != nil {
		return err
	}
	if ce != nil {
		return ce
	}
	if err = p.job.Link(tmp, receiptFile); err != nil {
		return err
	}
	return syncPageDir(p.job, ".")
}
func verifyPageTree(job *os.Root, id, expected string) error {
	dst, err := pageSubdir(job, "pages", false)
	if err != nil {
		return err
	}
	defer dst.Close()
	got, err := pageTreeDigest(dst, id, false)
	if err != nil {
		return err
	}
	if got != expected {
		return errors.New("published clone pages differ from their receipt")
	}
	return nil
}
func verifyPublishedPages(data, id string) error {
	job, err := pageJobRoot(data, id, false)
	if err != nil {
		return err
	}
	defer job.Close()
	digest, err := readPageReceipt(job)
	if err != nil {
		return err
	}
	return verifyPageTree(job, id, digest)
}
func (p *pageRestore) publish() error {
	if p.f != nil {
		return ErrInvalid
	}
	expected, err := pageTreeDigest(p.root, p.id, true)
	if err != nil {
		return err
	}
	_, err = p.job.Lstat("pages")
	exists := err == nil
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	saved, err := readPageReceipt(p.job)
	switch {
	case errors.Is(err, os.ErrNotExist):
		if exists {
			return errors.New("clone page directory already exists without this job's receipt")
		}
		if err = p.writeReceipt(expected); err != nil {
			return err
		}
	case err != nil:
		return err
	case saved != expected:
		return errors.New("clone page receipt conflicts with this snapshot")
	}
	if exists {
		if err = verifyPageTree(p.job, p.id, expected); err != nil {
			return err
		}
	} else {
		if err = p.job.Rename(p.stage, "pages"); err != nil {
			return err
		}
		p.stage = ""
	}
	return syncPageDir(p.job, ".")
}
