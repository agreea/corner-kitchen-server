package main

import (
	"code.google.com/p/go-uuid/uuid"
	"database/sql"
	_ "github.com/go-sql-driver/mysql"
	"log"
	"time"
)

type SessionManager struct {
	db *sql.DB
}

func NewSessionManager(server_config *Config) *SessionManager {
	t := new(SessionManager)

	// Set up database connection
	db, err := sql.Open("mysql", server_config.GetSqlURI())
	if err != nil {
		log.Fatal("NewSessionManager", "Failed to open database:", err)
	}
	t.db = db

	go t.stale_session_worker()

	return t
}

// Goroutine that scans the database for expired tokens and drops them
func (t *SessionManager) stale_session_worker() {
	for {
		_, err := t.db.Exec("DELETE FROM UserSession WHERE Expire_time < CURRENT_TIMESTAMP")
		if err != nil {
			log.Println("stale_session_worker", err)
		}
		time.Sleep(1 * time.Hour)
	}
}

// Create a session, add it to the cache and plug it into the DB.
func (t *SessionManager) CreateSessionForUser(uid int64) (string, error) {
	session_uuid := uuid.New()

	// Get the user's info
	user_data, err := GetUserById(t.db, uid)
	if err != nil {
		return "", err
	}

	// Create the session object and put it in the local cache
	UserSession := new(Session)
	UserSession.User = user_data
	UserSession.Expires = time.Now().Add(48 * time.Hour)

	// Store the token in the database
	_, err = t.db.Exec(`INSERT INTO  UserSession (
		Token, User_id, Expire_time ) VALUES (?, ?, ?)`, session_uuid, uid, UserSession.Expires)
	if err != nil {
		// This isn't a fatal error since the session will be known by this API
		// server, but the session will be lost if the api server is restarted.
		// Can also lead to premature expiry in highly available API clusters.
		log.Println("CreateSessionForUser", err)
	}

	return session_uuid, nil
}

// Deletes a session from the database and local cache
func (t *SessionManager) DestroySession(session_uuid string) error {
	_, err := t.db.Exec("DELETE FROM  UserSession WHERE Token = ?", session_uuid)
	return err
}

// Fetch the session specified by a UUID. Returns whether the session exists,
// the session (if it exists) and an error.
func (t *SessionManager) GetSession(session_uuid string) (session_exists bool, session *Session, err error) {
	err = nil

	// If it wasn't loaded into the cache, check if it's in the database.
	in_db, uid, expires, err := t.get_session_from_db(session_uuid)
	if err != nil {
		return false, nil, err
	}
	if in_db {
		// Load the session back into the cache and return it
		UserSession := new(Session)
		UserSession.User, err = GetUserById(t.db, uid)
		if err != nil {
			return false, nil, err
		}
		UserSession.Expires = expires
		return true, UserSession, nil
	}

	// If it isn't in cache or DB, return false.
	return false, nil, nil
}

// Check if a session exists in the database and is still valid.
// Returns three values - whether the token exists & is valid, the user id and
// an error.
func (t *SessionManager) get_session_from_db(session_uuid string) (exists bool, user_id int64, expire_time time.Time, err error) {

	rows, err := t.db.Query(`
		SELECT User_id, Expire_time
		FROM UserSession
		WHERE Token = ? AND Expire_time > CURRENT_TIMESTAMP()`, session_uuid)
	if err != nil {
		return false, 0, time.Now(), err
	}
	defer rows.Close()

	num_rows := 0
	for rows.Next() {
		num_rows++
		if err := rows.Scan(&user_id, &expire_time); err != nil {
			return false, 0, time.Now(), err
		}
	}
	if err := rows.Err(); err != nil {
		return false, 0, time.Now(), err
	}
	// If we got no rows, the session is invalid / expired.
	if num_rows == 0 {
		return false, 0, time.Now(), nil
	}

	// Otherwise, we got a valid token
	return true, user_id, expire_time, nil
}
