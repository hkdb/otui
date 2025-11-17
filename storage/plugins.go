package storage

import (
	"database/sql"
	"fmt"
	"path/filepath"
	"time"

	_ "modernc.org/sqlite"
)

type InstalledPlugin struct {
	ID            string
	Name          string
	Version       string
	InstallPath   string
	InstallMethod string
	ServerURL     string // For remote plugins
	AuthType      string // For remote plugins: "none", "headers", "oauth"
	Transport     string // For remote plugins: "sse" (default), "streamable-http"
	InstalledAt   time.Time
	UpdatedAt     time.Time
}

type PluginStorage struct {
	db *sql.DB
}

func NewPluginStorage(dataDir string) (*PluginStorage, error) {
	dbPath := filepath.Join(dataDir, "plugins.db")

	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}

	if err := db.Ping(); err != nil {
		return nil, fmt.Errorf("failed to ping database: %w", err)
	}

	storage := &PluginStorage{db: db}

	if err := storage.initialize(); err != nil {
		return nil, fmt.Errorf("failed to initialize database: %w", err)
	}

	return storage, nil
}

func (ps *PluginStorage) initialize() error {
	schema := `
	CREATE TABLE IF NOT EXISTS plugins (
		id TEXT PRIMARY KEY,
		name TEXT NOT NULL,
		version TEXT NOT NULL,
		install_path TEXT NOT NULL,
		install_method TEXT NOT NULL,
		server_url TEXT,
		auth_type TEXT,
		transport TEXT,
		installed_at DATETIME NOT NULL,
		updated_at DATETIME NOT NULL
	);
	CREATE INDEX IF NOT EXISTS idx_plugins_name ON plugins(name);
	`

	_, err := ps.db.Exec(schema)
	if err != nil {
		return err
	}

	// Migration: Add server_url and auth_type columns if they don't exist
	// This handles existing databases that were created before remote plugin support
	if err := ps.migrateSchema(); err != nil {
		return fmt.Errorf("schema migration failed: %w", err)
	}

	return nil
}

// migrateSchema adds missing columns to existing databases
func (ps *PluginStorage) migrateSchema() error {
	// Check if server_url column exists
	hasServerURL, err := ps.columnExists("plugins", "server_url")
	if err != nil {
		return fmt.Errorf("failed to check for server_url column: %w", err)
	}

	// Add server_url if missing
	switch {
	case !hasServerURL:
		_, err := ps.db.Exec(`ALTER TABLE plugins ADD COLUMN server_url TEXT DEFAULT ''`)
		if err != nil {
			return fmt.Errorf("failed to add server_url column: %w", err)
		}
	}

	// Check if auth_type column exists
	hasAuthType, err := ps.columnExists("plugins", "auth_type")
	if err != nil {
		return fmt.Errorf("failed to check for auth_type column: %w", err)
	}

	// Add auth_type if missing
	switch {
	case !hasAuthType:
		_, err := ps.db.Exec(`ALTER TABLE plugins ADD COLUMN auth_type TEXT DEFAULT ''`)
		if err != nil {
			return fmt.Errorf("failed to add auth_type column: %w", err)
		}
	}

	// Check if transport column exists
	hasTransport, err := ps.columnExists("plugins", "transport")
	if err != nil {
		return fmt.Errorf("failed to check for transport column: %w", err)
	}

	// Add transport if missing
	switch {
	case !hasTransport:
		_, err := ps.db.Exec(`ALTER TABLE plugins ADD COLUMN transport TEXT DEFAULT ''`)
		if err != nil {
			return fmt.Errorf("failed to add transport column: %w", err)
		}
	}

	return nil
}

// columnExists checks if a column exists in a table using PRAGMA table_info
func (ps *PluginStorage) columnExists(tableName, columnName string) (bool, error) {
	query := fmt.Sprintf("PRAGMA table_info(%s)", tableName)
	rows, err := ps.db.Query(query)
	if err != nil {
		return false, err
	}
	defer rows.Close()

	for rows.Next() {
		var cid int
		var name string
		var dataType string
		var notNull int
		var defaultValue interface{}
		var pk int

		err := rows.Scan(&cid, &name, &dataType, &notNull, &defaultValue, &pk)
		if err != nil {
			return false, err
		}

		switch {
		case name == columnName:
			return true, nil
		}
	}

	return false, rows.Err()
}

func (ps *PluginStorage) Save(plugin InstalledPlugin) error {
	query := `
	INSERT OR REPLACE INTO plugins (id, name, version, install_path, install_method, server_url, auth_type, transport, installed_at, updated_at)
	VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`

	_, err := ps.db.Exec(query,
		plugin.ID,
		plugin.Name,
		plugin.Version,
		plugin.InstallPath,
		plugin.InstallMethod,
		plugin.ServerURL,
		plugin.AuthType,
		plugin.Transport,
		plugin.InstalledAt,
		plugin.UpdatedAt,
	)

	return err
}

func (ps *PluginStorage) Load(id string) (*InstalledPlugin, error) {
	query := `
	SELECT id, name, version, install_path, install_method, server_url, auth_type, transport, installed_at, updated_at
	FROM plugins
	WHERE id = ?
	`

	var plugin InstalledPlugin
	err := ps.db.QueryRow(query, id).Scan(
		&plugin.ID,
		&plugin.Name,
		&plugin.Version,
		&plugin.InstallPath,
		&plugin.InstallMethod,
		&plugin.ServerURL,
		&plugin.AuthType,
		&plugin.Transport,
		&plugin.InstalledAt,
		&plugin.UpdatedAt,
	)

	if err == sql.ErrNoRows {
		return nil, nil
	}

	if err != nil {
		return nil, err
	}

	return &plugin, nil
}

func (ps *PluginStorage) List() ([]InstalledPlugin, error) {
	query := `
	SELECT id, name, version, install_path, install_method, server_url, auth_type, transport, installed_at, updated_at
	FROM plugins
	ORDER BY installed_at DESC
	`

	rows, err := ps.db.Query(query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var plugins []InstalledPlugin
	for rows.Next() {
		var plugin InstalledPlugin
		err := rows.Scan(
			&plugin.ID,
			&plugin.Name,
			&plugin.Version,
			&plugin.InstallPath,
			&plugin.InstallMethod,
			&plugin.ServerURL,
			&plugin.AuthType,
			&plugin.Transport,
			&plugin.InstalledAt,
			&plugin.UpdatedAt,
		)
		if err != nil {
			continue
		}
		plugins = append(plugins, plugin)
	}

	return plugins, nil
}

func (ps *PluginStorage) Delete(id string) error {
	query := `DELETE FROM plugins WHERE id = ?`
	_, err := ps.db.Exec(query, id)
	return err
}

func (ps *PluginStorage) IsInstalled(id string) bool {
	plugin, err := ps.Load(id)
	return err == nil && plugin != nil
}

// Update updates an existing plugin in the database
func (ps *PluginStorage) Update(plugin InstalledPlugin) error {
	updateSQL := `
		UPDATE plugins 
		SET name = ?, 
			version = ?, 
			install_path = ?,
			install_method = ?,
			server_url = ?,
			auth_type = ?,
			transport = ?,
			updated_at = ?
		WHERE id = ?
	`

	result, err := ps.db.Exec(updateSQL,
		plugin.Name,
		plugin.Version,
		plugin.InstallPath,
		plugin.InstallMethod,
		plugin.ServerURL,
		plugin.AuthType,
		plugin.Transport,
		plugin.UpdatedAt,
		plugin.ID,
	)

	if err != nil {
		return fmt.Errorf("failed to update plugin: %w", err)
	}

	// Check if any rows were affected
	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to check affected rows: %w", err)
	}

	if rows == 0 {
		return fmt.Errorf("plugin %s not found in database", plugin.ID)
	}

	return nil
}

func (ps *PluginStorage) Close() error {
	if ps.db != nil {
		return ps.db.Close()
	}
	return nil
}
