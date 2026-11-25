package config

import (
	"errors"
	"path/filepath"
	"testing"
)

func TestDirFor(t *testing.T) {
	home := func() (string, error) { return "/home/u", nil }
	env := func(vars map[string]string) func(string) string {
		return func(k string) string { return vars[k] }
	}

	tests := []struct {
		name string
		goos string
		vars map[string]string
		want string
	}{
		{"unix default", "linux", nil, filepath.Join("/home/u", ".config", "ssh-pve")},
		{"darwin default", "darwin", nil, filepath.Join("/home/u", ".config", "ssh-pve")},
		{"unix xdg", "linux", map[string]string{"XDG_CONFIG_HOME": "/xdg"}, filepath.Join("/xdg", "ssh-pve")},
		{"windows appdata", "windows", map[string]string{"APPDATA": "/appdata"}, filepath.Join("/appdata", "ssh-pve")},
		{
			"windows xdg wins over appdata", "windows",
			map[string]string{"XDG_CONFIG_HOME": "/xdg", "APPDATA": "/appdata"},
			filepath.Join("/xdg", "ssh-pve"),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := dirFor(tt.goos, env(tt.vars), home)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tt.want {
				t.Errorf("got %q, want %q", got, tt.want)
			}
		})
	}
}

func TestDirForErrors(t *testing.T) {
	noEnv := func(string) string { return "" }

	if _, err := dirFor("windows", noEnv, func() (string, error) { return "/home/u", nil }); err == nil {
		t.Error("windows without APPDATA: expected an error")
	}

	homeErr := errors.New("no home")
	if _, err := dirFor("linux", noEnv, func() (string, error) { return "", homeErr }); !errors.Is(err, homeErr) {
		t.Errorf("got %v, want %v", err, homeErr)
	}
}
