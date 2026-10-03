package database

import (
	"testing"
	"time"
)

func TestPoolConfigIsSizedForServerless(t *testing.T) {
	config, err := poolConfig(Options{DSN: "postgres://user:pass@localhost:5432/db", MaxConns: 3, MinConns: 0})
	if err != nil {
		t.Fatalf("pool config: %v", err)
	}
	if config.MaxConns != 3 || config.MinConns != 0 {
		t.Fatalf("pool size = %d/%d, want 3/0", config.MaxConns, config.MinConns)
	}
	if config.MaxConnIdleTime > 30*time.Second {
		t.Fatalf("idle time = %s, want idle connections released within 30s", config.MaxConnIdleTime)
	}
	if config.ConnConfig.RuntimeParams["TimeZone"] != "UTC" {
		t.Fatal("connections must be pinned to UTC")
	}
}

func TestPoolConfigRejectsInvalidSizes(t *testing.T) {
	for _, options := range []Options{
		{DSN: "postgres://localhost/db", MaxConns: 0},
		{DSN: "postgres://localhost/db", MaxConns: 2, MinConns: 3},
		{DSN: "postgres://localhost/db", MaxConns: 2, MinConns: -1},
	} {
		if _, err := poolConfig(options); err == nil {
			t.Fatalf("pool config %+v must be rejected", options)
		}
	}
}
