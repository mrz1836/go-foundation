package config

import (
	"errors"
	"fmt"
	"regexp"
)

var (
	// versionRegex matches semantic versions with optional 'v' prefix and git describe suffixes
	// Supports: 1.0.0, v1.0.0, 1.0.0-5-gabcdef, v1.0.0-5-gabcdef
	versionRegex = regexp.MustCompile(`^v?\d+\.\d+\.\d+(-\d+-g[a-f0-9]+)?$`)

	// regionRegex validates AWS region format (e.g., us-east-1, eu-west-2)
	regionRegex = regexp.MustCompile(`^[a-z]{2}-[a-z]+-\d+$`)

	// Static error variables for err113 compliance
	errMissingAppName           = errors.New("missing required field: ApplicationConfig.Name")
	errInvalidAppVersion        = errors.New("invalid version format: must follow semantic versioning (e.g., 1.0.0, v1.0.0)")
	errInvalidAppTimeout        = errors.New("ApplicationConfig.Timeout must be between 1 and 900 seconds")
	errMissingWriteDBHost       = errors.New("missing required field: WriteDatabaseConfig.Host")
	errMissingReadDBHost        = errors.New("missing required field: ReadDatabaseConfig.Host")
	errInvalidDBPort            = errors.New("DatabaseConfig.Port must be between 1024 and 65535")
	errMissingWriteDBName       = errors.New("missing required field: WriteDatabaseConfig.Database")
	errMissingReadDBName        = errors.New("missing required field: ReadDatabaseConfig.Database")
	errMissingWriteDBUsername   = errors.New("missing required field: WriteDatabaseConfig.Username")
	errMissingReadDBUsername    = errors.New("missing required field: ReadDatabaseConfig.Username")
	errInvalidDBSSLMode         = errors.New("DatabaseConfig.SSLMode must be one of: disable, require, verify-ca, verify-full")
	errInvalidDBMaxOpenConns    = errors.New("DatabaseConfig.MaxOpenConns must be >= 1")
	errInvalidDBMaxIdleConns    = errors.New("DatabaseConfig.MaxIdleConns must be >= 0 and <= MaxOpenConns")
	errInvalidDBConnMaxLife     = errors.New("DatabaseConfig.ConnMaxLifetime must be between 1 and 14 minutes")
	errInvalidLogLevel          = errors.New("LoggingConfig.Level must be one of: debug, info, warn, error")
	errInvalidLogFormat         = errors.New("LoggingConfig.Format must be one of: json, text")
	errMissingAWSRegion         = errors.New("missing required field: AWSConfig.Region")
	errInvalidAWSRegionFormat   = errors.New("invalid AWS region format (expected format: us-east-1, eu-west-2, etc.)")
	errInvalidConfigEnvironment = errors.New("Config.Environment must be one of: development, local, production, staging")
)

// Validate validates the ApplicationConfig and returns an error if invalid.
func (a *ApplicationConfig) Validate() error {
	if a.Name == "" {
		return errMissingAppName
	}
	// Version is optional in JSON config (will be set via environment variable at deploy time)
	// If provided, it must be valid
	if a.Version != "" && !versionRegex.MatchString(a.Version) {
		return fmt.Errorf("%w: got '%s'", errInvalidAppVersion, a.Version)
	}

	if a.Timeout < 1 || a.Timeout > 900 {
		return fmt.Errorf("%w: got %d", errInvalidAppTimeout, a.Timeout)
	}

	return nil
}

// isValidSSLMode checks whether the given SSL mode is valid for PostgreSQL connections.
func isValidSSLMode(mode string) bool {
	switch mode {
	case "disable", "require", "verify-ca", "verify-full":
		return true
	default:
		return false
	}
}

// validateDBConnectionFields validates database connection parameters.
func validateDBConnectionFields(host string, port int, database, username, sslMode string,
	errHost, errDatabase, errUsername error,
) error {
	if host == "" {
		return errHost
	}

	if port < 1024 || port > 65535 {
		return fmt.Errorf("%w: got %d", errInvalidDBPort, port)
	}

	if database == "" {
		return errDatabase
	}

	if username == "" {
		return errUsername
	}

	if !isValidSSLMode(sslMode) {
		return fmt.Errorf("%w: got '%s'", errInvalidDBSSLMode, sslMode)
	}

	return nil
}

// validateDBPoolingFields validates database connection pool parameters.
func validateDBPoolingFields(maxOpenConns, maxIdleConns, connMaxLifetime int) error {
	if maxOpenConns < 1 {
		return fmt.Errorf("%w: got %d", errInvalidDBMaxOpenConns, maxOpenConns)
	}

	if maxIdleConns < 0 || maxIdleConns > maxOpenConns {
		return fmt.Errorf("%w: got %d (MaxOpenConns: %d)", errInvalidDBMaxIdleConns, maxIdleConns, maxOpenConns)
	}

	if connMaxLifetime < 1 || connMaxLifetime > 14 {
		return fmt.Errorf("%w: got %d", errInvalidDBConnMaxLife, connMaxLifetime)
	}

	return nil
}

// Validate validates the WriteDatabaseConfig and returns an error if invalid.
func (d *WriteDatabaseConfig) Validate() error {
	if err := validateDBConnectionFields(d.Host, d.Port, d.Database, d.Username, d.SSLMode,
		errMissingWriteDBHost, errMissingWriteDBName, errMissingWriteDBUsername); err != nil {
		return err
	}

	return validateDBPoolingFields(d.MaxOpenConns, d.MaxIdleConns, d.ConnMaxLifetime)
}

// Validate validates the ReadDatabaseConfig and returns an error if invalid.
func (d *ReadDatabaseConfig) Validate() error {
	if err := validateDBConnectionFields(d.Host, d.Port, d.Database, d.Username, d.SSLMode,
		errMissingReadDBHost, errMissingReadDBName, errMissingReadDBUsername); err != nil {
		return err
	}

	return validateDBPoolingFields(d.MaxOpenConns, d.MaxIdleConns, d.ConnMaxLifetime)
}

// Validate validates the LoggingConfig and returns an error if invalid.
func (l *LoggingConfig) Validate() error {
	switch l.Level {
	case "debug", "info", "warn", "error":
	default:
		return fmt.Errorf("%w: got '%s'", errInvalidLogLevel, l.Level)
	}

	switch l.Format {
	case "json", "text":
	default:
		return fmt.Errorf("%w: got '%s'", errInvalidLogFormat, l.Format)
	}

	return nil
}

// Validate validates the AWSConfig and returns an error if invalid.
func (a *AWSConfig) Validate() error {
	if a.Region == "" {
		return errMissingAWSRegion
	}

	if !regionRegex.MatchString(a.Region) {
		return fmt.Errorf("%w: got '%s'", errInvalidAWSRegionFormat, a.Region)
	}

	return nil
}

// Validate validates the entire Config structure and all nested configurations.
// All validation errors are collected and returned together via errors.Join so
// operators can fix every issue in one pass rather than one at a time.
func (c *Config) Validate() error {
	var errs []error

	switch c.Environment {
	case "development", "local", "production", "staging":
	default:
		errs = append(errs, fmt.Errorf("%w: got '%s'", errInvalidConfigEnvironment, c.Environment))
	}

	if err := c.Application.Validate(); err != nil {
		errs = append(errs, fmt.Errorf("application config invalid: %w", err))
	}

	if err := c.WriteDatabase.Validate(); err != nil {
		errs = append(errs, fmt.Errorf("write database config invalid: %w", err))
	}

	if err := c.ReadDatabase.Validate(); err != nil {
		errs = append(errs, fmt.Errorf("read database config invalid: %w", err))
	}

	if err := c.Logging.Validate(); err != nil {
		errs = append(errs, fmt.Errorf("logging config invalid: %w", err))
	}

	if err := c.AWS.Validate(); err != nil {
		errs = append(errs, fmt.Errorf("aws config invalid: %w", err))
	}

	return errors.Join(errs...)
}
