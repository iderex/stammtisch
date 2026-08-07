// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026  iderex

// Package proof carries one deliberate defect so that the CodeQL job on this
// branch has something to find. It is not for merging. See the pull request
// body for what it proves and why the branch stays unmerged.
package proof

import (
	"net/http"
	"os/exec"
)

// Handler takes a value straight off an untrusted request and puts it in the
// argument list of a command it then runs. This is the shape the go/command-injection
// query is about, and it is the shape a signalling server gets wrong when it
// shells out to anything at all.
func Handler(w http.ResponseWriter, r *http.Request) {
	region := r.URL.Query().Get("region")
	out, err := exec.Command("sh", "-c", "ping -c 1 "+region).CombinedOutput()
	if err != nil {
		http.Error(w, "probe failed", http.StatusBadGateway)
		return
	}
	_, _ = w.Write(out)
}
