// commands/db_helpers.go
package commands

import (
	"database/sql"
	"errors"
	"time"
)

const IVY_DECIMALS = 1e9

func ensureUserExists(db *sql.DB, userID string) error {
	_, err := db.Exec("INSERT OR IGNORE INTO users (user_id) VALUES (?)", userID)
	return err
}

func getUserBalanceRaw(db *sql.DB, userID string) (uint64, error) {
	var balance uint64
	err := db.QueryRow("SELECT balance_raw FROM users WHERE user_id = ?", userID).Scan(&balance)
	return balance, err
}

func updateBalanceRaw(db *sql.DB, userID string, amountRaw int64) error {
	res, err := db.Exec("UPDATE users SET balance_raw = balance_raw + ? WHERE user_id = ?", amountRaw, userID)
	if err != nil {
		return err
	}
	aff, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if aff < 1 {
		return errors.New("no rows affected")
	}
	return nil
}

func transferFundsRaw(db *sql.DB, senderID, recipientID string, amountRaw uint64) error {
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	// Deduct from sender
	res, err := tx.Exec("UPDATE users SET balance_raw = balance_raw - ? WHERE user_id = ?", amountRaw, senderID)
	if err != nil {
		return err
	}
	aff, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if aff < 1 {
		return errors.New("no rows affected")
	}

	// Add to recipient
	res, err = tx.Exec("UPDATE users SET balance_raw = balance_raw + ? WHERE user_id = ?", amountRaw, recipientID)
	if err != nil {
		return err
	}
	aff, err = res.RowsAffected()
	if err != nil {
		return err
	}
	if aff < 1 {
		return errors.New("no rows affected")
	}

	return tx.Commit()
}

func createDeposit(db *sql.DB, depositID, userID string, amountRaw uint64) error {
	_, err := db.Exec(
		"INSERT INTO deposits (deposit_id, user_id, amount_raw, completed) VALUES (?, ?, ?, 0)",
		depositID,
		userID,
		amountRaw,
	)
	return err
}

func completeDeposit(db *sql.DB, depositID string) error {
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	var userID string
	var amountRaw uint64
	err = tx.QueryRow(
		"SELECT user_id, amount_raw FROM deposits WHERE deposit_id = ? AND completed = 0",
		depositID,
	).Scan(&userID, &amountRaw)
	if err != nil {
		return err
	}

	// Mark deposit as completed
	result, err := tx.Exec("UPDATE deposits SET completed = 1 WHERE deposit_id = ? AND completed = 0", depositID)
	if err != nil {
		return err
	}
	aff, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if aff < 1 {
		return errors.New("no rows affected")
	}

	// Credit user
	result, err = tx.Exec("UPDATE users SET balance_raw = balance_raw + ? WHERE user_id = ?", amountRaw, userID)
	if err != nil {
		return err
	}
	aff, err = result.RowsAffected()
	if err != nil {
		return err
	}
	if aff < 1 {
		return errors.New("no rows affected")
	}

	return tx.Commit()
}

// usage of oldBalanceRaw creates a compare-and-swap where it's impossible for a race condition
// to allow balance_raw to go negative :)
func createWithdrawal(db *sql.DB, withdrawID, userID string, oldBalanceRaw, amountRaw uint64, signature string) error {
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	// Debit user
	result, err := tx.Exec(
		"UPDATE users SET balance_raw = balance_raw - ? WHERE user_id = ? AND balance_raw = ?",
		amountRaw, userID, oldBalanceRaw,
	)
	if err != nil {
		return err
	}
	aff, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if aff < 1 {
		return errors.New("no rows affected")
	}

	// Create withdrawal record
	_, err = tx.Exec(
		"INSERT INTO withdrawals (withdraw_id, user_id, amount_raw, signature) VALUES (?, ?, ?, ?)",
		withdrawID,
		userID,
		amountRaw,
		signature,
	)
	if err != nil {
		return err
	}

	return tx.Commit()
}

// processRain handles the database transaction for distributing rain
// usage of senderBalanceRaw creates a compare-and-swap where it is impossible for a race condition
// to allow balance_raw to go negative :)
func processRain(db *sql.DB, senderID string, recipientIDs []string, totalAmountRaw uint64, senderBalanceRaw uint64) (uint64, error) {
	if len(recipientIDs) == 0 {
		return 0, errors.New("no recipients")
	}

	amountPerUserRaw := totalAmountRaw / uint64(len(recipientIDs))
	if amountPerUserRaw == 0 {
		return 0, errors.New("amount too small to distribute")
	}

	tx, err := db.Begin()
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()

	// Deduct from sender
	// Compare-and-swap
	result, err := tx.Exec("UPDATE users SET balance_raw = balance_raw - ? WHERE user_id = ? AND balance_raw = ?", totalAmountRaw, senderID, senderBalanceRaw)
	if err != nil {
		return 0, err
	}
	aff, err := result.RowsAffected()
	if err != nil {
		return 0, err
	}
	if aff < 1 {
		return 0, errors.New("sender not found")
	}

	// Ensure all recipients exist and credit them
	for _, recipientID := range recipientIDs {
		// Ensure user exists
		_, err := tx.Exec("INSERT OR IGNORE INTO users (user_id) VALUES (?)", recipientID)
		if err != nil {
			return 0, err
		}

		// Credit recipient
		_, err = tx.Exec("UPDATE users SET balance_raw = balance_raw + ? WHERE user_id = ?", amountPerUserRaw, recipientID)
		if err != nil {
			return 0, err
		}
	}

	if err = tx.Commit(); err != nil {
		return 0, err
	}

	return amountPerUserRaw, nil
}

// getActiveUsersForRain returns user IDs with activity score >= minScore for a given server
func getActiveUsersForRain(db *sql.DB, serverID string, minScore int) ([]string, error) {
	// prune activity entries that are too old
	threshold := time.Now().Unix() - 1800
	_, err := db.Exec(
		"DELETE FROM activity WHERE last_message_timestamp < ?",
		threshold,
	)
	if err != nil {
		return nil, err
	}

	rows, err := db.Query(`
        SELECT user_id
        FROM activity
        WHERE server_id = ? AND score >= ?
        ORDER BY score DESC
    `, serverID, minScore)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var activeUsers []string
	for rows.Next() {
		var userID string
		if err := rows.Scan(&userID); err != nil {
			return nil, err
		}
		activeUsers = append(activeUsers, userID)
	}

	return activeUsers, rows.Err()
}
