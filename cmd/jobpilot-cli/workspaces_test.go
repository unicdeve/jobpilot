package main

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestWorkspaceIDFromArgs(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want string
	}{
		{name: "absent", args: []string{"profiles", "list"}},
		{name: "separate value", args: []string{"--workspace", "workspace-1", "scouts", "list"}, want: "workspace-1"},
		{name: "equals value", args: []string{"scouts", "list", "--workspace=workspace-2"}, want: "workspace-2"},
		{name: "missing value", args: []string{"--workspace"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, workspaceIDFromArgs(tt.args))
		})
	}
}
