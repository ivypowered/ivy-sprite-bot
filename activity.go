package main

import (
	"database/sql"
	"time"

	"github.com/ivypowered/ivy-sprite-bot/constants"
)

// Add this function to handle activity score updates
func updateActivityScore(db *sql.DB, serverID, userID string) error {
	currentTime := time.Now().Unix()

	// First, try to get existing score and timestamp
	var score int
	var lastTimestamp int64
	err := db.QueryRow(`
        SELECT score, last_message_timestamp
        FROM activity
        WHERE server_id = ? AND user_id = ?
    `, serverID, userID).Scan(&score, &lastTimestamp)

	if err == sql.ErrNoRows {
		// New user, insert with score 1
		_, err = db.Exec(`
            INSERT OR IGNORE INTO activity (server_id, user_id, score, last_message_timestamp)
            VALUES (?, ?, 1, ?)
        `, serverID, userID, currentTime)
		return err
	} else if err != nil {
		return err
	}

	// Calculate delta in seconds
	delta := currentTime - lastTimestamp

	var newScore int
	if delta <= 90 {
		// Less than 90 seconds - don't change score
		newScore = score
	} else if delta > 90 && delta <= 1200 {
		// Between 90 seconds and 20 minutes - increase score (max 10)
		newScore = score + 1
		if newScore > constants.ACTIVITY_MAX {
			newScore = constants.ACTIVITY_MAX
		}
	} else if delta > 1200 && delta <= 1800 {
		// Between 20 and 30 minutes - decrease score (min 1)
		newScore = score - 1
		if newScore < 1 {
			newScore = 1
		}
	} else {
		// More than 30 minutes - reset to 1
		newScore = 1
	}

	// Update the record
	_, err = db.Exec(`
        UPDATE activity
        SET score = ?, last_message_timestamp = ?
        WHERE server_id = ? AND user_id = ?
    `, newScore, currentTime, serverID, userID)

	return err
}
