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
		installed_at DATETIME NOT NULL,
		updated_at DATETIME NOT NULL
	);
	CREATE INDEX IF NOT EXISTS idx_plugins_name ON plugins(name);
	`

	_, err := ps.db.Exec(schema)
	return err
}

func (ps *PluginStorage) Save(plugin InstalledPlugin) error {
	query := `
	INSERT OR REPLACE INTO plugins (id, name, version, install_path, install_method, installed_at, updated_at)
	VALUES (?, ?, ?, ?, ?, ?, ?)
	`

	_, err := ps.db.Exec(query,
		plugin.ID,
		plugin.Name,
		plugin.Version,
		plugin.InstallPath,
		plugin.InstallMethod,
		plugin.InstalledAt,
		plugin.UpdatedAt,
	)

	return err
}

func (ps *PluginStorage) Load(id string) (*InstalledPlugin, error) {
	query := `
	SELECT id, name, version, install_path, install_method, installed_at, updated_at
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
	SELECT id, name, version, install_path, install_method, installed_at, updated_at
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

func (ps *PluginStorage) Close() error {
	if ps.db != nil {
		return ps.db.Close()
	}
	return nil
}
