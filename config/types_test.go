package config_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/mrz1836/go-foundation/config"
)

func TestWriteDatabaseConfig_Getters(t *testing.T) {
	t.Parallel()

	d := &config.WriteDatabaseConfig{
		Host:            "write-host",
		Port:            5433,
		Database:        "testdb",
		Username:        "dbuser",
		Password:        "dbpass",
		SSLMode:         "require",
		MaxOpenConns:    20,
		MaxIdleConns:    10,
		ConnMaxLifetime: 7,
		Driver:          "pgx",
	}

	t.Run("GetHost", func(t *testing.T) { t.Parallel(); assert.Equal(t, "write-host", d.GetHost()) })
	t.Run("GetPort", func(t *testing.T) { t.Parallel(); assert.Equal(t, 5433, d.GetPort()) })
	t.Run("GetDatabase", func(t *testing.T) { t.Parallel(); assert.Equal(t, "testdb", d.GetDatabase()) })
	t.Run("GetUsername", func(t *testing.T) { t.Parallel(); assert.Equal(t, "dbuser", d.GetUsername()) })
	t.Run("GetPassword", func(t *testing.T) { t.Parallel(); assert.Equal(t, "dbpass", d.GetPassword()) })
	t.Run("GetSSLMode", func(t *testing.T) { t.Parallel(); assert.Equal(t, "require", d.GetSSLMode()) })
	t.Run("GetMaxOpenConns", func(t *testing.T) { t.Parallel(); assert.Equal(t, 20, d.GetMaxOpenConns()) })
	t.Run("GetMaxIdleConns", func(t *testing.T) { t.Parallel(); assert.Equal(t, 10, d.GetMaxIdleConns()) })
	t.Run("GetConnMaxLifetime", func(t *testing.T) { t.Parallel(); assert.Equal(t, 7, d.GetConnMaxLifetime()) })
	t.Run("GetDriver", func(t *testing.T) { t.Parallel(); assert.Equal(t, "pgx", d.GetDriver()) })
}

func TestReadDatabaseConfig_Getters(t *testing.T) {
	t.Parallel()

	d := &config.ReadDatabaseConfig{
		Host:            "read-host",
		Port:            5434,
		Database:        "readdb",
		Username:        "readuser",
		Password:        "readpass",
		SSLMode:         "verify-full",
		MaxOpenConns:    15,
		MaxIdleConns:    8,
		ConnMaxLifetime: 3,
		Driver:          "pgx",
	}

	t.Run("GetHost", func(t *testing.T) { t.Parallel(); assert.Equal(t, "read-host", d.GetHost()) })
	t.Run("GetPort", func(t *testing.T) { t.Parallel(); assert.Equal(t, 5434, d.GetPort()) })
	t.Run("GetDatabase", func(t *testing.T) { t.Parallel(); assert.Equal(t, "readdb", d.GetDatabase()) })
	t.Run("GetUsername", func(t *testing.T) { t.Parallel(); assert.Equal(t, "readuser", d.GetUsername()) })
	t.Run("GetPassword", func(t *testing.T) { t.Parallel(); assert.Equal(t, "readpass", d.GetPassword()) })
	t.Run("GetSSLMode", func(t *testing.T) { t.Parallel(); assert.Equal(t, "verify-full", d.GetSSLMode()) })
	t.Run("GetMaxOpenConns", func(t *testing.T) { t.Parallel(); assert.Equal(t, 15, d.GetMaxOpenConns()) })
	t.Run("GetMaxIdleConns", func(t *testing.T) { t.Parallel(); assert.Equal(t, 8, d.GetMaxIdleConns()) })
	t.Run("GetConnMaxLifetime", func(t *testing.T) { t.Parallel(); assert.Equal(t, 3, d.GetConnMaxLifetime()) })
	t.Run("GetDriver", func(t *testing.T) { t.Parallel(); assert.Equal(t, "pgx", d.GetDriver()) })
}

func TestWriteDatabaseConfig_ConnectionString(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		password string
		contains []string
		absent   []string
	}{
		{
			name:     "no password",
			password: "",
			absent:   []string{"password="},
		},
		{
			name:     "simple password",
			password: "simple123",
			contains: []string{"password='simple123'"},
		},
		{
			name:     "password with space",
			password: "pass word",
			contains: []string{"password='pass word'"},
		},
		{
			name:     "password with single quote",
			password: "pass'word",
			contains: []string{"password='pass''word'"},
		},
		{
			name:     "password with backslash",
			password: `pass\word`,
			contains: []string{`password='pass\\word'`},
		},
		{
			name:     "space and single quote combined",
			password: "p w's",
			contains: []string{"password='p w''s'"},
		},
		{
			name:     "all standard DSN fields present",
			password: "",
			contains: []string{"host=", "port=", "dbname=", "user=", "sslmode=", "connect_timeout=5"},
		},
		{
			name:     "fields with special characters are escaped",
			password: "",
			contains: []string{"host='localhost'", "dbname='testdb'", "user='user'", "sslmode='disable'"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			d := &config.WriteDatabaseConfig{
				Host:     "localhost",
				Port:     5432,
				Database: "testdb",
				Username: "user",
				Password: tt.password,
				SSLMode:  "disable",
			}

			cs := d.ConnectionString()
			for _, s := range tt.contains {
				assert.Contains(t, cs, s, "expected %q to contain %q", cs, s)
			}

			for _, s := range tt.absent {
				assert.NotContains(t, cs, s, "expected %q to not contain %q", cs, s)
			}
		})
	}
}

func TestReadDatabaseConfig_ConnectionString(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		password string
		contains []string
		absent   []string
	}{
		{
			name:     "no password",
			password: "",
			absent:   []string{"password="},
		},
		{
			name:     "simple password",
			password: "mypass",
			contains: []string{"password='mypass'"},
		},
		{
			name:     "password with space",
			password: "my pass",
			contains: []string{"password='my pass'"},
		},
		{
			name:     "standard DSN fields present",
			password: "",
			contains: []string{"host=", "port=", "dbname=", "user=", "sslmode=", "connect_timeout=5"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			d := &config.ReadDatabaseConfig{
				Host:     "read-host",
				Port:     5432,
				Database: "readdb",
				Username: "readuser",
				Password: tt.password,
				SSLMode:  "require",
			}

			cs := d.ConnectionString()
			for _, s := range tt.contains {
				assert.Contains(t, cs, s, "expected %q to contain %q", cs, s)
			}

			for _, s := range tt.absent {
				assert.NotContains(t, cs, s, "expected %q to not contain %q", cs, s)
			}
		})
	}
}

func TestDatabaseConfig_Interface(_ *testing.T) {
	// Compile-time interface satisfaction check
	var (
		_ config.DatabaseConfig = &config.WriteDatabaseConfig{}
		_ config.DatabaseConfig = &config.ReadDatabaseConfig{}
	)
}

func TestWriteDatabaseConfig_ConnMaxLifetimeDuration(t *testing.T) {
	t.Parallel()

	d := &config.WriteDatabaseConfig{ConnMaxLifetime: 7}
	assert.Equal(t, 7*time.Minute, d.ConnMaxLifetimeDuration())
}

func TestReadDatabaseConfig_ConnMaxLifetimeDuration(t *testing.T) {
	t.Parallel()

	d := &config.ReadDatabaseConfig{ConnMaxLifetime: 3}
	assert.Equal(t, 3*time.Minute, d.ConnMaxLifetimeDuration())
}

func TestConnectionString_DSNInjectionPrevention(t *testing.T) {
	t.Parallel()

	d := &config.WriteDatabaseConfig{
		Host:     "evil host=attacker.com",
		Port:     5432,
		Database: "testdb",
		Username: "admin host=evil.com",
		SSLMode:  "disable",
	}
	cs := d.ConnectionString()

	// Injected values should be quoted, preventing parameter injection
	assert.Contains(t, cs, "host='evil host=attacker.com'")
	assert.Contains(t, cs, "user='admin host=evil.com'")
}
