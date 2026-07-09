package config

import (
	"fmt"
	"os"
)

type Config struct {
	Server    ServerConfig
	Database  DatabaseConfig
	Log       LogConfig
	Reporting ReportingConfig
	Services  ServicesConfig
	Scheduler SchedulerConfig
	Report    ReportConfig
	SMTP      SMTPConfig
}

type ServerConfig struct {
	Port string
}

type DatabaseConfig struct {
	Host     string
	Port     string
	User     string
	Password string
	Name     string
	SSLMode  string
}

type LogConfig struct {
	Level string
}

type ReportingConfig struct {
	// Timezone defines the calendar used for scheduling and period math. Must
	// match the transactions service so month windows line up.
	Timezone string
}

type ServicesConfig struct {
	TransactionsBaseURL string
	EventsBaseURL       string
	AIInternalBaseURL   string
	// CallbackBaseURL is the address the events container uses to call back
	// into this service (a container-network address in prod, e.g.
	// http://reports:8080).
	CallbackBaseURL string
}

type SchedulerConfig struct {
	// Enabled runs the ticker loop. Set false to run API/UI only (e.g. staging).
	Enabled bool
}

type ReportConfig struct {
	Language string
	Currency string
}

type SMTPConfig struct {
	Host     string
	Port     string
	Username string
	Password string
	From     string
}

// Configured reports whether enough SMTP settings are present to send email.
func (s SMTPConfig) Configured() bool {
	return s.Host != "" && s.Port != "" && s.From != ""
}

func Load() (*Config, error) {
	cfg := &Config{
		Server: ServerConfig{
			Port: getEnv("SERVER_PORT", "8080"),
		},
		Database: DatabaseConfig{
			Host:     getEnv("DB_HOST", "localhost"),
			Port:     getEnv("DB_PORT", "5432"),
			User:     getEnv("DB_USER", "reports"),
			Password: getEnv("DB_PASSWORD", "reports_dev_password"),
			Name:     getEnv("DB_NAME", "reports_db"),
			SSLMode:  getEnv("DB_SSLMODE", "disable"),
		},
		Log: LogConfig{
			Level: getEnv("LOG_LEVEL", "info"),
		},
		Reporting: ReportingConfig{
			Timezone: getEnv("REPORTING_TIMEZONE", "America/Sao_Paulo"),
		},
		Services: ServicesConfig{
			TransactionsBaseURL: getEnv("TRANSACTIONS_BASE_URL", "http://localhost:1235/api/v2"),
			EventsBaseURL:       getEnv("EVENTS_BASE_URL", "http://localhost:3000"),
			AIInternalBaseURL:   getEnv("AI_INTERNAL_BASE_URL", "http://localhost:3005"),
			CallbackBaseURL:     getEnv("REPORTS_CALLBACK_BASE_URL", "http://localhost:8080"),
		},
		Scheduler: SchedulerConfig{
			Enabled: getEnvBool("SCHEDULER_ENABLED", true),
		},
		Report: ReportConfig{
			Language: getEnv("REPORT_LANGUAGE", "en"),
			Currency: getEnv("REPORT_CURRENCY", "BRL"),
		},
		SMTP: SMTPConfig{
			Host:     getEnv("SMTP_HOST", ""),
			Port:     getEnv("SMTP_PORT", ""),
			Username: getEnv("SMTP_USERNAME", ""),
			Password: getEnv("SMTP_PASSWORD", ""),
			From:     getEnv("SMTP_FROM", ""),
		},
	}

	return cfg, nil
}

func (c *DatabaseConfig) DSN() string {
	return fmt.Sprintf(
		"host=%s port=%s user=%s password=%s dbname=%s sslmode=%s",
		c.Host, c.Port, c.User, c.Password, c.Name, c.SSLMode,
	)
}

func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

func getEnvBool(key string, defaultValue bool) bool {
	value := os.Getenv(key)
	switch value {
	case "":
		return defaultValue
	case "1", "true", "TRUE", "True", "yes", "YES":
		return true
	case "0", "false", "FALSE", "False", "no", "NO":
		return false
	default:
		return defaultValue
	}
}
