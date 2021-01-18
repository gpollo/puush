package database

import (
	"database/sql"
	"errors"
	"fmt"
	"math/rand"
	"os"

	"time"

	_ "github.com/lib/pq"
)

type Database struct {
	handle *sql.DB
}

func ConnectDatabase() (Database, error) {
	user := os.Getenv("PUUSH_POSTGRESQL_USER")
	if user == "" {
		return Database{}, errors.New("PUUSH_POSTGRESQL_USER is not defined")
	}

	pass := os.Getenv("PUUSH_POSTGRESQL_PASS")
	if user == "" {
		return Database{}, errors.New("PUUSH_POSTGRESQL_PASS is not defined")
	}

	host := os.Getenv("PUUSH_POSTGRESQL_HOST")
	if user == "" {
		return Database{}, errors.New("PUUSH_POSTGRESQL_HOST is not defined")
	}

	name := os.Getenv("PUUSH_POSTGRESQL_DATABASE")
	if user == "" {
		return Database{}, errors.New("PUUSH_POSTGRESQL_DATABASE is not defined")
	}

	connectionStringTemplate := "postgres://%s:%s@%s/%s?sslmode=disable"
	connectionString := fmt.Sprintf(connectionStringTemplate, user, pass, host, name)

	handle, err := sql.Open("postgres", connectionString)
	if err != nil {
		return Database{}, err
	}

	database := Database{handle: handle}

	err = database.setup()
	if err != nil {
		return Database{}, err
	}

	rand.Seed(time.Now().UnixNano())
	return database, nil
}

func (db *Database) setup() error {
	sql := "CREATE EXTENSION IF NOT EXISTS pgcrypto;\n"
	sql += "\n"
	sql += "CREATE TABLE IF NOT EXISTS Session(\n"
	sql += "    key UUID UNIQUE NOT NULL DEFAULT gen_random_uuid(),\n"
	sql += "    since TIMESTAMP NOT NULL DEFAULT now(),\n"
	sql += "\n"
	sql += "    PRIMARY KEY (key)\n"
	sql += ");\n"
	sql += "\n"
	sql += "CREATE TABLE IF NOT EXISTS File(\n"
	sql += "    id VARCHAR(32) UNIQUE NOT NULL,\n"
	sql += "    session UUID NOT NULL,\n"
	sql += "    filename VARCHAR(128) NOT NULL,\n"
	sql += "    since TIMESTAMP NOT NULL DEFAULT now(),\n"
	sql += "\n"
	sql += "    PRIMARY KEY (id),\n"
	sql += "    FOREIGN KEY (session) REFERENCES Session(key) ON DELETE CASCADE\n"
	sql += ");"

	rows, err := db.handle.Query(sql)
	if err != nil {
		return err
	}
	defer rows.Close()

	return nil
}

func (db *Database) checkIfFileIDExists(id string) (bool, error) {
	sql := "SELECT COUNT(*) AS count FROM File WHERE id = $1;"

	rows, err := db.handle.Query(sql, id)
	if err != nil {
		return true, err
	}
	defer rows.Close()

	if !rows.Next() {
		return true, errors.New("Unexpected number of row returned")
	}

	var count int
	if err = rows.Scan(&count); err != nil {
		return true, err
	}

	return (count == 1), nil
}

func (db *Database) randomFileIDLen(n int) string {
	runes := []rune("abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789")

	id := make([]rune, n)
	for i := range id {
		id[i] = runes[rand.Intn(len(runes))]
	}

	return string(id)
}

func (db *Database) generateRandomFileID() (string, error) {
	len := 3
	attempt := 0

	for {
		id := db.randomFileIDLen(len)
		exists, err := db.checkIfFileIDExists(id)
		if err != nil {
			return "", err
		}

		if !exists {
			return id, nil
		}

		attempt += 1
		if attempt > 20 {
			len += 1
			attempt = 0
		}
	}
}

func (db *Database) DoesSessionExist(key string) (bool, error) {
	sql := "SELECT COUNT(*) AS count FROM Session WHERE key = $1;"

	rows, err := db.handle.Query(sql, key)
	if err != nil {
		return true, err
	}
	defer rows.Close()

	if !rows.Next() {
		return true, errors.New("Unexpected number of row returned")
	}

	var count int
	if err = rows.Scan(&count); err != nil {
		return true, err
	}

	return (count == 1), nil
}

func (db *Database) AddFile(sessionKey, filename string) (string, error) {
	sessionExists, err := db.DoesSessionExist(sessionKey)
	if err != nil {
		return "", err
	}

	if !sessionExists {
		return "", errors.New("Session does not exists")
	}

	fileID, err := db.generateRandomFileID()
	if err != nil {
		return "", err
	}

	sql := "INSERT INTO File (id, session, filename) VALUES ($1, $2, $3);"

	rows, err := db.handle.Query(sql, fileID, sessionKey, filename)
	if err != nil {
		return "", err
	}
	defer rows.Close()

	return fileID, nil
}

func (db *Database) DeleteFile(sessionKey, fileID string) error {
	if _, err := db.GetFile(sessionKey, fileID); err != nil {
		return err
	}

	sql := "DELETE FROM File WHERE id = $1 AND session = $2;"

	rows, err := db.handle.Query(sql, fileID, sessionKey)
	if err != nil {
		return err
	}
	defer rows.Close()

	return nil
}

func (db *Database) GetFile(sessionKey, fileID string) (string, error) {
	var rows *sql.Rows
	var err error

	if sessionKey == "" {
		sql := "SELECT filename FROM File WHERE id = $1;"
		rows, err = db.handle.Query(sql, fileID)
	} else {
		sql := "SELECT filename FROM File WHERE id = $1 AND session = $2;"
		rows, err = db.handle.Query(sql, fileID, sessionKey)
	}

	if err != nil {
		return "", err
	}
	defer rows.Close()

	if !rows.Next() {
		return "", nil
	}

	var filename string
	if err = rows.Scan(&filename); err != nil {
		return "", err
	}

	return filename, nil
}

func (db *Database) AddSession() (string, error) {
	sql := "INSERT INTO Session DEFAULT VALUES RETURNING key;"

	rows, err := db.handle.Query(sql)
	if err != nil {
		return "", err
	}
	defer rows.Close()

	if !rows.Next() {
		return "", errors.New("Unexpected number of row returned")
	}

	var key string
	if err = rows.Scan(&key); err != nil {
		return "", err
	}

	return key, nil
}

type UploadedFile struct {
	Id    string `json:"id"`
	Name  string `json:"name"`
	Since string `json:"since"`
}

func (db *Database) ListFiles(sessionKey string) ([]UploadedFile, error) {
	sql := "SELECT id,filename,since FROM File WHERE session = $1;"

	rows, err := db.handle.Query(sql, sessionKey)
	if err != nil {
		return []UploadedFile{}, err
	}
	defer rows.Close()

	files := []UploadedFile{}
	for rows.Next() {
		var id string
		var name string
		var since string

		if err := rows.Scan(&id, &name, &since); err != nil {
			return []UploadedFile{}, err
		}

		file := UploadedFile{Id: id, Name: name, Since: since}
		files = append(files, file)
	}

	return files, nil
}
