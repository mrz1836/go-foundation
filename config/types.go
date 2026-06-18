// Package config provides a lightweight, type-safe configuration layer for
// serverless and HTTP services. It exposes generic, domain-agnostic
// configuration types plus validation and DSN helpers.
//
// The "env" struct tags carry generic, prefix-less variable names (for example
// "DB_WRITE_HOST"). Consuming services own the project-specific environment
// prefix (for example "MYAPP_") and prepend it when binding environment
// variables, so no project naming lives in this module.
package config

import (
	"fmt"
	"strings"
	"time"
)

// Config is the root configuration structure containing all generic application
// settings. Services that need additional, project-specific configuration embed
// this type and add their own fields.
type Config struct {
	Environment   string              `json:"environment"`
	Application   ApplicationConfig   `json:"application"`
	WriteDatabase WriteDatabaseConfig `json:"write_database"`
	ReadDatabase  ReadDatabaseConfig  `json:"read_database"`
	Logging       LoggingConfig       `json:"logging"`
	AWS           AWSConfig           `json:"aws"`
}

// ApplicationConfig contains application-level settings.
type ApplicationConfig struct {
	Name        string `json:"name" env:"APP_NAME"`
	Version     string `json:"version" env:"APP_VERSION"`
	Commit      string `json:"commit" env:"APP_COMMIT"`
	Environment string `json:"environment"`
	Debug       bool   `json:"debug" env:"APP_DEBUG"`
	Timeout     int    `json:"timeout" env:"APP_TIMEOUT"`
}

// WriteDatabaseConfig contains database connection configuration for the primary/write PostgreSQL instance.
type WriteDatabaseConfig struct {
	Host            string `json:"host" env:"DB_WRITE_HOST"`
	Port            int    `json:"port" env:"DB_WRITE_PORT"`
	Database        string `json:"database" env:"DB_WRITE_NAME"`
	Username        string `json:"username" env:"DB_WRITE_USERNAME"`
	Password        string `json:"password" env:"DB_WRITE_PASSWORD"`
	SSLMode         string `json:"ssl_mode" env:"DB_WRITE_SSL_MODE"`
	MaxOpenConns    int    `json:"max_open_conns" env:"DB_WRITE_MAX_OPEN_CONNS"`
	MaxIdleConns    int    `json:"max_idle_conns" env:"DB_WRITE_MAX_IDLE_CONNS"`
	ConnMaxLifetime int    `json:"conn_max_lifetime" env:"DB_WRITE_CONN_MAX_LIFETIME"`
	Driver          string `json:"driver" env:"DB_WRITE_DRIVER"`
}

// ReadDatabaseConfig contains database connection configuration for the read replica PostgreSQL instance.
type ReadDatabaseConfig struct {
	Host            string `json:"host" env:"DB_READ_HOST"`
	Port            int    `json:"port" env:"DB_READ_PORT"`
	Database        string `json:"database" env:"DB_READ_NAME"`
	Username        string `json:"username" env:"DB_READ_USERNAME"`
	Password        string `json:"password" env:"DB_READ_PASSWORD"`
	SSLMode         string `json:"ssl_mode" env:"DB_READ_SSL_MODE"`
	MaxOpenConns    int    `json:"max_open_conns" env:"DB_READ_MAX_OPEN_CONNS"`
	MaxIdleConns    int    `json:"max_idle_conns" env:"DB_READ_MAX_IDLE_CONNS"`
	ConnMaxLifetime int    `json:"conn_max_lifetime" env:"DB_READ_CONN_MAX_LIFETIME"`
	Driver          string `json:"driver" env:"DB_READ_DRIVER"`
}

// DatabaseConfig is a common interface for database configuration.
// Both WriteDatabaseConfig and ReadDatabaseConfig implement this through their methods.
type DatabaseConfig interface {
	ConnectionString() string
	GetHost() string
	GetPort() int
	GetDatabase() string
	GetUsername() string
	GetPassword() string
	GetSSLMode() string
	GetMaxOpenConns() int
	GetMaxIdleConns() int
	GetConnMaxLifetime() int
	GetDriver() string
}

// LoggingConfig contains logging configuration for structured JSON logging.
type LoggingConfig struct {
	Level            string `json:"level" env:"LOG_LEVEL"`
	Format           string `json:"format" env:"LOG_FORMAT"`
	IncludeTimestamp bool   `json:"include_timestamp" env:"LOG_INCLUDE_TIMESTAMP"`
	IncludeCaller    bool   `json:"include_caller" env:"LOG_INCLUDE_CALLER"`
}

// AWSConfig contains AWS-specific configuration.
type AWSConfig struct {
	Region string `json:"region" env:"AWS_REGION"`
}

// ConnectionString builds a PostgreSQL connection string from WriteDatabaseConfig.
// Returns a DSN suitable for use with pgx driver.
func (d *WriteDatabaseConfig) ConnectionString() string {
	return buildConnectionString(d.Host, d.Port, d.Database, d.Username, d.Password, d.SSLMode)
}

// GetHost returns the database host.
func (d *WriteDatabaseConfig) GetHost() string { return d.Host }

// GetPort returns the database port.
func (d *WriteDatabaseConfig) GetPort() int { return d.Port }

// GetDatabase returns the database name.
func (d *WriteDatabaseConfig) GetDatabase() string { return d.Database }

// GetUsername returns the database username.
func (d *WriteDatabaseConfig) GetUsername() string { return d.Username }

// GetPassword returns the database password.
func (d *WriteDatabaseConfig) GetPassword() string { return d.Password }

// GetSSLMode returns the SSL mode.
func (d *WriteDatabaseConfig) GetSSLMode() string { return d.SSLMode }

// GetMaxOpenConns returns the maximum number of open connections.
func (d *WriteDatabaseConfig) GetMaxOpenConns() int { return d.MaxOpenConns }

// GetMaxIdleConns returns the maximum number of idle connections.
func (d *WriteDatabaseConfig) GetMaxIdleConns() int { return d.MaxIdleConns }

// GetConnMaxLifetime returns the connection maximum lifetime in minutes.
func (d *WriteDatabaseConfig) GetConnMaxLifetime() int { return d.ConnMaxLifetime }

// GetDriver returns the database driver name.
func (d *WriteDatabaseConfig) GetDriver() string { return d.Driver }

// ConnMaxLifetimeDuration returns the connection max lifetime as a time.Duration.
func (d *WriteDatabaseConfig) ConnMaxLifetimeDuration() time.Duration {
	return time.Duration(d.ConnMaxLifetime) * time.Minute
}

// ConnectionString builds a PostgreSQL connection string from ReadDatabaseConfig.
// Returns a DSN suitable for use with pgx driver.
func (d *ReadDatabaseConfig) ConnectionString() string {
	return buildConnectionString(d.Host, d.Port, d.Database, d.Username, d.Password, d.SSLMode)
}

// GetHost returns the database host.
func (d *ReadDatabaseConfig) GetHost() string { return d.Host }

// GetPort returns the database port.
func (d *ReadDatabaseConfig) GetPort() int { return d.Port }

// GetDatabase returns the database name.
func (d *ReadDatabaseConfig) GetDatabase() string { return d.Database }

// GetUsername returns the database username.
func (d *ReadDatabaseConfig) GetUsername() string { return d.Username }

// GetPassword returns the database password.
func (d *ReadDatabaseConfig) GetPassword() string { return d.Password }

// GetSSLMode returns the SSL mode.
func (d *ReadDatabaseConfig) GetSSLMode() string { return d.SSLMode }

// GetMaxOpenConns returns the maximum number of open connections.
func (d *ReadDatabaseConfig) GetMaxOpenConns() int { return d.MaxOpenConns }

// GetMaxIdleConns returns the maximum number of idle connections.
func (d *ReadDatabaseConfig) GetMaxIdleConns() int { return d.MaxIdleConns }

// GetConnMaxLifetime returns the connection maximum lifetime in minutes.
func (d *ReadDatabaseConfig) GetConnMaxLifetime() int { return d.ConnMaxLifetime }

// GetDriver returns the database driver name.
func (d *ReadDatabaseConfig) GetDriver() string { return d.Driver }

// ConnMaxLifetimeDuration returns the connection max lifetime as a time.Duration.
func (d *ReadDatabaseConfig) ConnMaxLifetimeDuration() time.Duration {
	return time.Duration(d.ConnMaxLifetime) * time.Minute
}

// buildConnectionString builds a PostgreSQL connection string from the given parameters.
// All values are escaped via escapeDSNValue to prevent DSN parameter injection.
func buildConnectionString(host string, port int, database, username, password, sslMode string) string {
	dsn := fmt.Sprintf("host=%s port=%d dbname=%s user=%s sslmode=%s connect_timeout=5",
		escapeDSNValue(host), port, escapeDSNValue(database),
		escapeDSNValue(username), escapeDSNValue(sslMode))

	// Include password if set
	if password != "" {
		dsn += fmt.Sprintf(" password=%s", escapeDSNValue(password))
	}

	return dsn
}

// escapeDSNValue escapes a value for use in a libpq keyword/value connection string.
// Always quotes the value to safely handle any special characters (spaces,
// equals signs, single quotes, backslashes) per the libpq connection string spec.
func escapeDSNValue(s string) string {
	escaped := strings.ReplaceAll(s, "\\", "\\\\")
	escaped = strings.ReplaceAll(escaped, "'", "''")

	return "'" + escaped + "'"
}
