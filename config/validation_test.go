package config_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mrz1836/go-foundation/config"
)

// validWriteDB returns a valid WriteDatabaseConfig for use as a test base.
func validWriteDB() config.WriteDatabaseConfig {
	return config.WriteDatabaseConfig{
		Host:            "localhost",
		Port:            5432,
		Database:        "app_testdb",
		Username:        "testuser",
		SSLMode:         "disable",
		MaxOpenConns:    10,
		MaxIdleConns:    5,
		ConnMaxLifetime: 5,
		Driver:          "pgx",
	}
}

// validReadDB returns a valid ReadDatabaseConfig for use as a test base.
func validReadDB() config.ReadDatabaseConfig {
	return config.ReadDatabaseConfig{
		Host:            "localhost",
		Port:            5432,
		Database:        "app_testdb",
		Username:        "testuser",
		SSLMode:         "disable",
		MaxOpenConns:    10,
		MaxIdleConns:    5,
		ConnMaxLifetime: 5,
		Driver:          "pgx",
	}
}

const testAppName = "myapp"

func TestApplicationConfig_Validate(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		cfg     config.ApplicationConfig
		wantErr bool
		errMsg  string
	}{
		{
			name:    "valid with name version timeout",
			cfg:     config.ApplicationConfig{Name: testAppName, Version: "1.0.0", Timeout: 30},
			wantErr: false,
		},
		{
			name:    "valid with empty version (optional)",
			cfg:     config.ApplicationConfig{Name: testAppName, Version: "", Timeout: 30},
			wantErr: false,
		},
		{
			name:    "valid with v-prefixed version",
			cfg:     config.ApplicationConfig{Name: testAppName, Version: "v1.0.0", Timeout: 30},
			wantErr: false,
		},
		{
			name:    "valid with git-describe suffix",
			cfg:     config.ApplicationConfig{Name: testAppName, Version: "1.2.3-5-gabcdef", Timeout: 30},
			wantErr: false,
		},
		{
			name:    "valid timeout boundary 1",
			cfg:     config.ApplicationConfig{Name: testAppName, Timeout: 1},
			wantErr: false,
		},
		{
			name:    "valid timeout boundary 900",
			cfg:     config.ApplicationConfig{Name: testAppName, Timeout: 900},
			wantErr: false,
		},
		{
			name:    "invalid empty name",
			cfg:     config.ApplicationConfig{Name: "", Timeout: 30},
			wantErr: true,
			errMsg:  "ApplicationConfig.Name",
		},
		{
			name:    "invalid version not-a-version",
			cfg:     config.ApplicationConfig{Name: testAppName, Version: "not-a-version", Timeout: 30},
			wantErr: true,
			errMsg:  "invalid version format",
		},
		{
			name:    "invalid version only two parts",
			cfg:     config.ApplicationConfig{Name: testAppName, Version: "1.0", Timeout: 30},
			wantErr: true,
			errMsg:  "invalid version format",
		},
		{
			name:    "invalid timeout 0",
			cfg:     config.ApplicationConfig{Name: testAppName, Timeout: 0},
			wantErr: true,
			errMsg:  "ApplicationConfig.Timeout",
		},
		{
			name:    "invalid timeout 901",
			cfg:     config.ApplicationConfig{Name: testAppName, Timeout: 901},
			wantErr: true,
			errMsg:  "ApplicationConfig.Timeout",
		},
		{
			name:    "invalid timeout negative",
			cfg:     config.ApplicationConfig{Name: testAppName, Timeout: -1},
			wantErr: true,
			errMsg:  "ApplicationConfig.Timeout",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			err := tt.cfg.Validate()
			if tt.wantErr {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.errMsg)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestWriteDatabaseConfig_Validate_Connection(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		modify  func(d *config.WriteDatabaseConfig)
		wantErr bool
		errMsg  string
	}{
		{
			name:    "valid base config",
			modify:  func(_ *config.WriteDatabaseConfig) {},
			wantErr: false,
		},
		{
			name:    "empty host",
			modify:  func(d *config.WriteDatabaseConfig) { d.Host = "" },
			wantErr: true,
			errMsg:  "WriteDatabaseConfig.Host",
		},
		{
			name:    "port below minimum 1023",
			modify:  func(d *config.WriteDatabaseConfig) { d.Port = 1023 },
			wantErr: true,
			errMsg:  "DatabaseConfig.Port",
		},
		{
			name:    "port at minimum boundary 1024",
			modify:  func(d *config.WriteDatabaseConfig) { d.Port = 1024 },
			wantErr: false,
		},
		{
			name:    "port at maximum boundary 65535",
			modify:  func(d *config.WriteDatabaseConfig) { d.Port = 65535 },
			wantErr: false,
		},
		{
			name:    "port above maximum 65536",
			modify:  func(d *config.WriteDatabaseConfig) { d.Port = 65536 },
			wantErr: true,
			errMsg:  "DatabaseConfig.Port",
		},
		{
			name:    "empty database",
			modify:  func(d *config.WriteDatabaseConfig) { d.Database = "" },
			wantErr: true,
			errMsg:  "WriteDatabaseConfig.Database",
		},
		{
			name:    "empty username",
			modify:  func(d *config.WriteDatabaseConfig) { d.Username = "" },
			wantErr: true,
			errMsg:  "WriteDatabaseConfig.Username",
		},
		{
			name:    "invalid ssl mode",
			modify:  func(d *config.WriteDatabaseConfig) { d.SSLMode = "invalid" },
			wantErr: true,
			errMsg:  "DatabaseConfig.SSLMode",
		},
		{
			name:    "valid ssl mode disable",
			modify:  func(d *config.WriteDatabaseConfig) { d.SSLMode = "disable" },
			wantErr: false,
		},
		{
			name:    "valid ssl mode require",
			modify:  func(d *config.WriteDatabaseConfig) { d.SSLMode = "require" },
			wantErr: false,
		},
		{
			name:    "valid ssl mode verify-ca",
			modify:  func(d *config.WriteDatabaseConfig) { d.SSLMode = "verify-ca" },
			wantErr: false,
		},
		{
			name:    "valid ssl mode verify-full",
			modify:  func(d *config.WriteDatabaseConfig) { d.SSLMode = "verify-full" },
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			d := validWriteDB()
			tt.modify(&d)

			err := d.Validate()
			if tt.wantErr {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.errMsg)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestWriteDatabaseConfig_Validate_Pooling(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name         string
		maxOpenConns int
		maxIdleConns int
		connMaxLife  int
		wantErr      bool
		errMsg       string
	}{
		{
			name:         "MaxOpenConns 0",
			maxOpenConns: 0, maxIdleConns: 0, connMaxLife: 5,
			wantErr: true, errMsg: "DatabaseConfig.MaxOpenConns",
		},
		{
			name:         "MaxOpenConns negative",
			maxOpenConns: -1, maxIdleConns: 0, connMaxLife: 5,
			wantErr: true, errMsg: "DatabaseConfig.MaxOpenConns",
		},
		{
			name:         "MaxOpenConns 1 boundary valid",
			maxOpenConns: 1, maxIdleConns: 0, connMaxLife: 1,
			wantErr: false,
		},
		{
			name:         "MaxIdleConns negative",
			maxOpenConns: 10, maxIdleConns: -1, connMaxLife: 5,
			wantErr: true, errMsg: "DatabaseConfig.MaxIdleConns",
		},
		{
			name:         "MaxIdleConns exceeds MaxOpenConns",
			maxOpenConns: 5, maxIdleConns: 6, connMaxLife: 5,
			wantErr: true, errMsg: "DatabaseConfig.MaxIdleConns",
		},
		{
			name:         "MaxIdleConns equals MaxOpenConns valid",
			maxOpenConns: 5, maxIdleConns: 5, connMaxLife: 5,
			wantErr: false,
		},
		{
			name:         "MaxIdleConns 0 valid",
			maxOpenConns: 10, maxIdleConns: 0, connMaxLife: 5,
			wantErr: false,
		},
		{
			name:         "ConnMaxLifetime 0",
			maxOpenConns: 10, maxIdleConns: 5, connMaxLife: 0,
			wantErr: true, errMsg: "DatabaseConfig.ConnMaxLifetime",
		},
		{
			name:         "ConnMaxLifetime 1 boundary valid",
			maxOpenConns: 10, maxIdleConns: 5, connMaxLife: 1,
			wantErr: false,
		},
		{
			name:         "ConnMaxLifetime 14 boundary valid",
			maxOpenConns: 10, maxIdleConns: 5, connMaxLife: 14,
			wantErr: false,
		},
		{
			name:         "ConnMaxLifetime 15 exceeds max",
			maxOpenConns: 10, maxIdleConns: 5, connMaxLife: 15,
			wantErr: true, errMsg: "DatabaseConfig.ConnMaxLifetime",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			d := validWriteDB()
			d.MaxOpenConns = tt.maxOpenConns
			d.MaxIdleConns = tt.maxIdleConns
			d.ConnMaxLifetime = tt.connMaxLife

			err := d.Validate()
			if tt.wantErr {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.errMsg)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestWriteDatabaseConfig_Validate_WarningPaths(t *testing.T) {
	t.Parallel()

	t.Run("non-standard port 5433 produces no error", func(t *testing.T) {
		t.Parallel()

		d := validWriteDB()
		d.Port = 5433
		require.NoError(t, d.Validate())
	})

	t.Run("database without app prefix produces no error", func(t *testing.T) {
		t.Parallel()

		d := validWriteDB()
		d.Database = "mydb"
		require.NoError(t, d.Validate())
	})

	t.Run("both non-standard port and db prefix produce no error", func(t *testing.T) {
		t.Parallel()

		d := validWriteDB()
		d.Port = 5433
		d.Database = "mydb"
		require.NoError(t, d.Validate())
	})
}

func TestReadDatabaseConfig_Validate_Connection(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		modify  func(d *config.ReadDatabaseConfig)
		wantErr bool
		errMsg  string
	}{
		{
			name:    "valid base config",
			modify:  func(_ *config.ReadDatabaseConfig) {},
			wantErr: false,
		},
		{
			name:    "empty host",
			modify:  func(d *config.ReadDatabaseConfig) { d.Host = "" },
			wantErr: true,
			errMsg:  "ReadDatabaseConfig.Host",
		},
		{
			name:    "port below minimum 1023",
			modify:  func(d *config.ReadDatabaseConfig) { d.Port = 1023 },
			wantErr: true,
			errMsg:  "DatabaseConfig.Port",
		},
		{
			name:    "port at minimum boundary 1024",
			modify:  func(d *config.ReadDatabaseConfig) { d.Port = 1024 },
			wantErr: false,
		},
		{
			name:    "port at maximum boundary 65535",
			modify:  func(d *config.ReadDatabaseConfig) { d.Port = 65535 },
			wantErr: false,
		},
		{
			name:    "port above maximum 65536",
			modify:  func(d *config.ReadDatabaseConfig) { d.Port = 65536 },
			wantErr: true,
			errMsg:  "DatabaseConfig.Port",
		},
		{
			name:    "empty database",
			modify:  func(d *config.ReadDatabaseConfig) { d.Database = "" },
			wantErr: true,
			errMsg:  "ReadDatabaseConfig.Database",
		},
		{
			name:    "empty username",
			modify:  func(d *config.ReadDatabaseConfig) { d.Username = "" },
			wantErr: true,
			errMsg:  "ReadDatabaseConfig.Username",
		},
		{
			name:    "invalid ssl mode",
			modify:  func(d *config.ReadDatabaseConfig) { d.SSLMode = "invalid" },
			wantErr: true,
			errMsg:  "DatabaseConfig.SSLMode",
		},
		{
			name:    "valid ssl mode disable",
			modify:  func(d *config.ReadDatabaseConfig) { d.SSLMode = "disable" },
			wantErr: false,
		},
		{
			name:    "valid ssl mode require",
			modify:  func(d *config.ReadDatabaseConfig) { d.SSLMode = "require" },
			wantErr: false,
		},
		{
			name:    "valid ssl mode verify-ca",
			modify:  func(d *config.ReadDatabaseConfig) { d.SSLMode = "verify-ca" },
			wantErr: false,
		},
		{
			name:    "valid ssl mode verify-full",
			modify:  func(d *config.ReadDatabaseConfig) { d.SSLMode = "verify-full" },
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			d := validReadDB()
			tt.modify(&d)

			err := d.Validate()
			if tt.wantErr {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.errMsg)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestReadDatabaseConfig_Validate_Pooling(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name         string
		maxOpenConns int
		maxIdleConns int
		connMaxLife  int
		wantErr      bool
		errMsg       string
	}{
		{
			name:         "MaxOpenConns 0",
			maxOpenConns: 0, maxIdleConns: 0, connMaxLife: 5,
			wantErr: true, errMsg: "DatabaseConfig.MaxOpenConns",
		},
		{
			name:         "MaxOpenConns negative",
			maxOpenConns: -1, maxIdleConns: 0, connMaxLife: 5,
			wantErr: true, errMsg: "DatabaseConfig.MaxOpenConns",
		},
		{
			name:         "MaxOpenConns 1 boundary valid",
			maxOpenConns: 1, maxIdleConns: 0, connMaxLife: 1,
			wantErr: false,
		},
		{
			name:         "MaxIdleConns negative",
			maxOpenConns: 10, maxIdleConns: -1, connMaxLife: 5,
			wantErr: true, errMsg: "DatabaseConfig.MaxIdleConns",
		},
		{
			name:         "MaxIdleConns exceeds MaxOpenConns",
			maxOpenConns: 5, maxIdleConns: 6, connMaxLife: 5,
			wantErr: true, errMsg: "DatabaseConfig.MaxIdleConns",
		},
		{
			name:         "MaxIdleConns equals MaxOpenConns valid",
			maxOpenConns: 5, maxIdleConns: 5, connMaxLife: 5,
			wantErr: false,
		},
		{
			name:         "MaxIdleConns 0 valid",
			maxOpenConns: 10, maxIdleConns: 0, connMaxLife: 5,
			wantErr: false,
		},
		{
			name:         "ConnMaxLifetime 0",
			maxOpenConns: 10, maxIdleConns: 5, connMaxLife: 0,
			wantErr: true, errMsg: "DatabaseConfig.ConnMaxLifetime",
		},
		{
			name:         "ConnMaxLifetime 1 boundary valid",
			maxOpenConns: 10, maxIdleConns: 5, connMaxLife: 1,
			wantErr: false,
		},
		{
			name:         "ConnMaxLifetime 14 boundary valid",
			maxOpenConns: 10, maxIdleConns: 5, connMaxLife: 14,
			wantErr: false,
		},
		{
			name:         "ConnMaxLifetime 15 exceeds max",
			maxOpenConns: 10, maxIdleConns: 5, connMaxLife: 15,
			wantErr: true, errMsg: "DatabaseConfig.ConnMaxLifetime",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			d := validReadDB()
			d.MaxOpenConns = tt.maxOpenConns
			d.MaxIdleConns = tt.maxIdleConns
			d.ConnMaxLifetime = tt.connMaxLife

			err := d.Validate()
			if tt.wantErr {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.errMsg)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestReadDatabaseConfig_Validate_WarningPaths(t *testing.T) {
	t.Parallel()

	t.Run("non-standard port 5433 produces no error", func(t *testing.T) {
		t.Parallel()

		d := validReadDB()
		d.Port = 5433
		require.NoError(t, d.Validate())
	})

	t.Run("database without app prefix produces no error", func(t *testing.T) {
		t.Parallel()

		d := validReadDB()
		d.Database = "mydb"
		require.NoError(t, d.Validate())
	})

	t.Run("both non-standard port and db prefix produce no error", func(t *testing.T) {
		t.Parallel()

		d := validReadDB()
		d.Port = 5433
		d.Database = "mydb"
		require.NoError(t, d.Validate())
	})
}

func TestLoggingConfig_Validate(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		level   string
		format  string
		wantErr bool
		errMsg  string
	}{
		{name: "debug json", level: "debug", format: "json", wantErr: false},
		{name: "info json", level: "info", format: "json", wantErr: false},
		{name: "warn text", level: "warn", format: "text", wantErr: false},
		{name: "error text", level: "error", format: "text", wantErr: false},
		{name: "debug text", level: "debug", format: "text", wantErr: false},
		{name: "info text", level: "info", format: "text", wantErr: false},
		{name: "warn json", level: "warn", format: "json", wantErr: false},
		{name: "error json", level: "error", format: "json", wantErr: false},
		{
			name: "invalid level", level: "trace", format: "json",
			wantErr: true, errMsg: "LoggingConfig.Level",
		},
		{
			name: "empty level", level: "", format: "json",
			wantErr: true, errMsg: "LoggingConfig.Level",
		},
		{
			name: "invalid format", level: "info", format: "xml",
			wantErr: true, errMsg: "LoggingConfig.Format",
		},
		{
			name: "empty format", level: "info", format: "",
			wantErr: true, errMsg: "LoggingConfig.Format",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			l := &config.LoggingConfig{Level: tt.level, Format: tt.format}

			err := l.Validate()
			if tt.wantErr {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.errMsg)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestAWSConfig_Validate(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		region  string
		wantErr bool
		errMsg  string
	}{
		{name: "us-east-1", region: "us-east-1", wantErr: false},
		{name: "eu-west-2", region: "eu-west-2", wantErr: false},
		{name: "ap-southeast-1", region: "ap-southeast-1", wantErr: false},
		{name: "ap-northeast-2", region: "ap-northeast-2", wantErr: false},
		{name: "sa-east-1", region: "sa-east-1", wantErr: false},
		{
			name: "empty region", region: "",
			wantErr: true, errMsg: "AWSConfig.Region",
		},
		{
			name: "no digit suffix", region: "us-east",
			wantErr: true, errMsg: "invalid AWS region format",
		},
		{
			name: "uppercase", region: "US-EAST-1",
			wantErr: true, errMsg: "invalid AWS region format",
		},
		{
			name: "no hyphen separator", region: "useast1",
			wantErr: true, errMsg: "invalid AWS region format",
		},
		{
			name: "invalid format with extra parts", region: "us-east-west-1",
			wantErr: true, errMsg: "invalid AWS region format",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			a := &config.AWSConfig{Region: tt.region}

			err := a.Validate()
			if tt.wantErr {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.errMsg)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestConfig_Validate(t *testing.T) {
	t.Parallel()

	t.Run("all valid environments pass", func(t *testing.T) {
		t.Parallel()

		for _, env := range []string{"development", "local", "production", "staging"} {
			t.Run(env, func(t *testing.T) {
				t.Parallel()

				cfg := newValidConfig()
				cfg.Environment = env
				require.NoError(t, cfg.Validate())
			})
		}
	})

	t.Run("test environment is rejected", func(t *testing.T) {
		t.Parallel()

		cfg := newValidConfig()
		cfg.Environment = "test"
		err := cfg.Validate()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "Config.Environment")
	})

	t.Run("empty environment is rejected", func(t *testing.T) {
		t.Parallel()

		cfg := newValidConfig()
		cfg.Environment = ""
		err := cfg.Validate()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "Config.Environment")
	})

	t.Run("invalid application config returns labeled error", func(t *testing.T) {
		t.Parallel()

		cfg := newValidConfig()
		cfg.Application.Name = ""
		err := cfg.Validate()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "application config invalid")
	})

	t.Run("invalid write database config returns labeled error", func(t *testing.T) {
		t.Parallel()

		cfg := newValidConfig()
		cfg.WriteDatabase.Host = ""
		err := cfg.Validate()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "write database config invalid")
	})

	t.Run("invalid read database config returns labeled error", func(t *testing.T) {
		t.Parallel()

		cfg := newValidConfig()
		cfg.ReadDatabase.Host = ""
		err := cfg.Validate()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "read database config invalid")
	})

	t.Run("invalid logging config returns labeled error", func(t *testing.T) {
		t.Parallel()

		cfg := newValidConfig()
		cfg.Logging.Level = ""
		err := cfg.Validate()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "logging config invalid")
	})

	t.Run("invalid aws config returns labeled error", func(t *testing.T) {
		t.Parallel()

		cfg := newValidConfig()
		cfg.AWS.Region = ""
		err := cfg.Validate()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "aws config invalid")
	})

	t.Run("multiple errors collected via errors.Join", func(t *testing.T) {
		t.Parallel()

		cfg := newValidConfig()
		cfg.Environment = "invalid"
		cfg.Application.Name = "" // also invalid
		err := cfg.Validate()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "Config.Environment")
		assert.Contains(t, err.Error(), "application config invalid")
	})

	t.Run("write and read database errors both reported", func(t *testing.T) {
		t.Parallel()

		cfg := newValidConfig()
		cfg.WriteDatabase.Host = "" // invalid write
		cfg.ReadDatabase.Host = ""  // also invalid read
		err := cfg.Validate()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "write database config invalid")
		assert.Contains(t, err.Error(), "read database config invalid")
	})
}
