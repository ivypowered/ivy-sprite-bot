package commands

import (
	"database/sql"
	"errors"
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
