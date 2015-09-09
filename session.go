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
	UserSession.Expires = time.Now().Add(60 * 24 * time.Hour)

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

func (t *SessionManager) CreateSessionForGuest(uid int64, token_expires int) (string, error) {
	session_uuid := uuid.New()
	guest_session, err := t.GetGuestSessionById(uid)
	if err != nil {
		return "Getting session by id", err
	}
	if guest_session != "" {
		return guest_session, err
	}
	// Get the user's info
	guest_data, err := GetGuestById(t.db, uid)
	if err != nil {
		return "getting guest by id", err
	}
	// Create the session object and put it in the local cache
	GuestSession := new(KitchenSession)
	GuestSession.Guest = guest_data
	GuestSession.Expires = time.Now().Add((token_expires - 100) * time.Second)

	// Store the token in the database
	_, err = t.db.Exec(`INSERT INTO  GuestSession (
		Token, Guest_id, Expires ) VALUES (?, ?, ?)`, session_uuid, uid, GuestSession.Expires)
	if err != nil {
		// 	// This isn't a fatal error since the session will be known by this API
		// 	// server, but the session will be lost if the api server is restarted.
		// 	// Can also lead to premature expiry in highly available API clusters.
		log.Println("CreateSessionForGuest", err)
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

func (t *SessionManager) GetGuestSession(session_uuid string) (session_exists bool, session *KitchenSession, err error) {
	err = nil
	in_db, uid, expires, err := t.get_guest_session_from_db(session_uuid)
	if err != nil {
		return false, nil, err
	}
	if in_db {
		GuestSession := new(KitchenSession)
		GuestSession.Guest, err = GetGuestById(t.db, uid)
		if err != nil {
			return false, nil, err
		}
		GuestSession.Expires = expires
		return true, GuestSession, nil
	} else {
		return false, nil, nil
	}
}

func (t *SessionManager) GetGuestSessionById(guest_id int64) (session string, err error) {
	err = nil
	// note: currently in_db always == false and session always == ""
	in_db, session, err := t.get_guest_session_from_db_by_id(guest_id)
	if err != nil {
		return "", err
	} else if in_db {
		return session, nil
	} else {
		return "", nil
	}
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

func (t *SessionManager) get_guest_session_from_db(session_uuid string) (exists bool, user_id int64, expire_time time.Time, err error) {

	rows, err := t.db.Query(`
		SELECT Guest_id, Expires
		FROM GuestSession
		WHERE Token = ? AND Expires > CURRENT_TIMESTAMP()`, session_uuid)
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

// returns true with a session value if there is a valid session for that user
func (t *SessionManager) get_guest_session_from_db_by_id(guest_id int64) (exists bool, session string, err error) {

	row, err := t.db.Query(`
		SELECT Token
		FROM GuestSession
		WHERE Guest_id = ? AND Expires > CURRENT_TIMESTAMP()`, guest_id)
	if err != nil {
		return false, "", err
	}
	defer row.Close()
	// if err := row.Scan(&session); err != nil {
	// 	return false, "", err
	// }
	// Otherwise, we got a valid token
	return true, session, nil
}
