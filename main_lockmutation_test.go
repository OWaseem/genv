package main

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/ks1686/genv/internal/genvfile"
)

// LockMutation must be held across every lock read-modify-write, and the lock
// re-read inside it.
//
// The interleaving mirrors the reported failure: the scheduled worker takes the
// mutation lock, reads a snapshot, and writes that snapshot back at the end.
// An append landing in that window is erased by the worker's stale write, and
// the next apply reinstalls the package. Holding the lock makes the append
// wait and then re-read the worker's result instead.
//
// The worker waits on appendDone but cannot require it: with the fix in place
// the append is correctly blocked until the worker releases, so the wait times
// out. Without the fix the append completes immediately and the worker's stale
// write erases it.
func TestAppendLockEntry_MergesWithConcurrentWriter(t *testing.T) {
	lockPath := filepath.Join(t.TempDir(), "genv.lock.json")
	if err := genvfile.WriteLock(lockPath, &genvfile.LockFile{Packages: []genvfile.LockedPackage{
		{ID: "doomed", Manager: "brew", PkgName: "doomed"},
	}}); err != nil {
		t.Fatalf("seed lock: %v", err)
	}

	readDone := make(chan struct{})
	appendDone := make(chan struct{})
	workerFinished := make(chan error, 1)

	go func() {
		unlock, err := genvfile.LockMutation(lockPath)
		if err != nil {
			workerFinished <- err
			return
		}
		defer unlock()
		snapshot, err := genvfile.ReadLock(lockPath)
		if err != nil {
			workerFinished <- err
			return
		}
		close(readDone)
		select {
		case <-appendDone:
		case <-time.After(300 * time.Millisecond):
		}
		snapshot.Packages = filterOut(snapshot.Packages, "doomed")
		workerFinished <- genvfile.WriteLock(lockPath, snapshot)
	}()

	<-readDone // the worker now holds the lock and has its stale snapshot

	if code := appendLockEntry(lockPath, genvfile.LockedPackage{ID: "fresh", Manager: "brew", PkgName: "fresh"}, "macos"); code != exitOK {
		t.Fatalf("appendLockEntry code = %d", code)
	}
	close(appendDone)
	if err := <-workerFinished; err != nil {
		t.Fatalf("worker: %v", err)
	}

	lf, err := genvfile.ReadLock(lockPath)
	if err != nil {
		t.Fatalf("read lock: %v", err)
	}
	var ids []string
	for _, p := range lf.Packages {
		ids = append(ids, p.ID)
	}
	if len(ids) != 1 || ids[0] != "fresh" {
		t.Fatalf("lock ids = %v, want [fresh] (the worker's removal kept, the append not erased)", ids)
	}
}

func filterOut(pkgs []genvfile.LockedPackage, id string) []genvfile.LockedPackage {
	out := pkgs[:0:0]
	for _, p := range pkgs {
		if p.ID != id {
			out = append(out, p)
		}
	}
	return out
}
