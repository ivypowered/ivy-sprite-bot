// db/db.go
package db

import (
	"database/sql"
	"errors"
	"time"

	"github.com/ivypowered/ivy-sprite-bot/constants"
	_ "github.com/mattn/go-sqlite3"
)

type Deposit struct {
	DepositID string
	UserID    string
	Timestamp int64
	AmountRaw uint64
	Completed bool
}

type Withdrawal struct {
	WithdrawID string
	UserID     string
	Timestamp  int64
	AmountRaw  uint64
	Signature  string
}

type Database struct {
	inner *sql.DB
}

// New creates and initializes a new database connection
func New(path string) (Database, error) {
	sqlDB, err := sql.Open("sqlite3", path)
	if err != nil {
		return Database{}, err
	}

	db := Database{inner: sqlDB}

	// Initialize tables
	if err := db.initTables(); err != nil {
		sqlDB.Close()
		return Database{}, err
	}

	return db, nil
}

// Close closes the database connection
func (db Database) Close() error {
	return db.inner.Close()
}

// initTables creates all necessary tables and indexes
func (db Database) initTables() error {
	queries := []string{
		`CREATE TABLE IF NOT EXISTS users (
			user_id TEXT PRIMARY KEY,
			balance_raw INTEGER DEFAULT 0
		);`,
		`CREATE TABLE IF NOT EXISTS deposits (
			deposit_id TEXT PRIMARY KEY,
			user_id TEXT NOT NULL,
			timestamp INTEGER NOT NULL DEFAULT (strftime('%s', 'now')),
			amount_raw INTEGER NOT NULL,
			completed INTEGER NOT NULL DEFAULT 0
		);`,
		`CREATE INDEX IF NOT EXISTS idx_deposit_user ON deposits(user_id);`,
		`CREATE TABLE IF NOT EXISTS withdrawals (
			withdraw_id TEXT PRIMARY KEY,
			user_id TEXT NOT NULL,
			timestamp INTEGER NOT NULL DEFAULT (strftime('%s', 'now')),
			amount_raw INTEGER NOT NULL,
			signature TEXT NOT NULL
		);`,
		`CREATE INDEX IF NOT EXISTS idx_withdrawal_user ON withdrawals(user_id);`,
		`CREATE TABLE IF NOT EXISTS activity (
			server_id TEXT NOT NULL,
			user_id TEXT NOT NULL,
			score INTEGER DEFAULT 1,
			last_message_timestamp INTEGER NOT NULL DEFAULT (strftime('%s', 'now')),
			PRIMARY KEY (server_id, user_id)
		);`,
		`CREATE INDEX IF NOT EXISTS idx_activity_server ON activity(server_id);`,
		`CREATE TABLE IF NOT EXISTS rain_channels (
            server_id TEXT NOT NULL,
            channel_id TEXT NOT NULL,
            PRIMARY KEY (server_id, channel_id)
        );`,
	}

	for _, query := range queries {
		if _, err := db.inner.Exec(query); err != nil {
			return err
		}
	}

	return nil
}

func (db Database) EnsureUserExists(userID string) error {
	_, err := db.inner.Exec("INSERT OR IGNORE INTO users (user_id) VALUES (?)", userID)
	return err
}

func (db Database) GetUserBalanceRaw(userID string) (uint64, error) {
	var balance uint64
	err := db.inner.QueryRow("SELECT balance_raw FROM users WHERE user_id = ?", userID).Scan(&balance)
	return balance, err
}

func (db Database) IsUserExtant(userID string) (bool, error) {
	_, err := db.GetUserBalanceRaw(userID)
	if err == sql.ErrNoRows {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return true, nil
}

func (db Database) UpdateBalanceRaw(userID string, amountRaw int64) error {
	res, err := db.inner.Exec("UPDATE users SET balance_raw = balance_raw + ? WHERE user_id = ?", amountRaw, userID)
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

func (db Database) TransferFundsRaw(senderID, recipientID string, amountRaw uint64) error {
	tx, err := db.inner.Begin()
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

func (db Database) CreateDeposit(depositID, userID string, amountRaw uint64) error {
	_, err := db.inner.Exec(
		"INSERT INTO deposits (deposit_id, user_id, amount_raw, completed) VALUES (?, ?, ?, 0)",
		depositID,
		userID,
		amountRaw,
	)
	return err
}

func (db Database) CompleteDeposit(depositID string) error {
	tx, err := db.inner.Begin()
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

func (db Database) CreateWithdrawal(withdrawID, userID string, oldBalanceRaw, amountRaw uint64, signature string) error {
	tx, err := db.inner.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	// Debit user with compare-and-swap
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

func (db Database) ProcessRain(senderID string, recipientIDs []string, totalAmountRaw uint64, senderBalanceRaw uint64) (uint64, error) {
	if len(recipientIDs) == 0 {
		return 0, errors.New("no recipients")
	}

	amountPerUserRaw := totalAmountRaw / uint64(len(recipientIDs))
	if amountPerUserRaw == 0 {
		return 0, errors.New("amount too small to distribute")
	}

	tx, err := db.inner.Begin()
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()

	// Deduct from sender with compare-and-swap
	result, err := tx.Exec("UPDATE users SET balance_raw = balance_raw - ? WHERE user_id = ? AND balance_raw = ?",
		totalAmountRaw, senderID, senderBalanceRaw)
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

func (db Database) GetActiveUsersForRain(serverID string, minScore int) ([]string, error) {
	// Prune activity entries that are too old
	threshold := time.Now().Unix() - constants.ACTIVITY_DELTA_RESET
	_, err := db.inner.Exec(
		"DELETE FROM activity WHERE last_message_timestamp < ?",
		threshold,
	)
	if err != nil {
		return nil, err
	}

	rows, err := db.inner.Query(`
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

// Activity-related methods
func (db Database) UpdateActivityScore(serverID, userID string) error {
	currentTime := time.Now().Unix()

	// First, try to get existing score and timestamp
	var score int
	var lastTimestamp int64
	err := db.inner.QueryRow(`
		SELECT score, last_message_timestamp
		FROM activity
		WHERE server_id = ? AND user_id = ?
	`, serverID, userID).Scan(&score, &lastTimestamp)

	if err == sql.ErrNoRows {
		// New user, insert with score 1
		_, err = db.inner.Exec(`
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
	if delta < constants.ACTIVITY_DELTA_MIN {
		// Less than 90 seconds - don't change score
		newScore = score
	} else if delta >= constants.ACTIVITY_DELTA_MIN && delta <= constants.ACTIVITY_DELTA_MAX {
		// Between 90 seconds and 20 minutes - increase score (max 10)
		newScore = score + 1
		if newScore > constants.ACTIVITY_MAX {
			newScore = constants.ACTIVITY_MAX
		}
	} else if delta > constants.ACTIVITY_DELTA_MAX && delta <= constants.ACTIVITY_DELTA_RESET {
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
	_, err = db.inner.Exec(`
		UPDATE activity
		SET score = ?, last_message_timestamp = ?
		WHERE server_id = ? AND user_id = ?
	`, newScore, currentTime, serverID, userID)

	return err
}

// FindDepositByPrefix finds a deposit by ID prefix for a user
func (db *Database) FindDepositByPrefix(userID, depositIDPrefix string) (string, uint64, int, error) {
	var fullDepositID string
	var amountRaw uint64
	var completed int

	err := db.inner.QueryRow(
		"SELECT deposit_id, amount_raw, completed FROM deposits WHERE user_id = ? AND deposit_id LIKE ? ORDER BY timestamp DESC LIMIT 1",
		userID,
		depositIDPrefix+"%",
	).Scan(&fullDepositID, &amountRaw, &completed)

	return fullDepositID, amountRaw, completed, err
}

// ListDeposits returns recent deposits for a user
func (db *Database) ListDeposits(userID string, limit int) ([]Deposit, error) {
	rows, err := db.inner.Query(
		"SELECT deposit_id, user_id, timestamp, amount_raw, completed FROM deposits WHERE user_id = ? ORDER BY timestamp DESC LIMIT ?",
		userID, limit,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var deposits []Deposit
	for rows.Next() {
		var d Deposit
		var completed int
		err := rows.Scan(&d.DepositID, &d.UserID, &d.Timestamp, &d.AmountRaw, &completed)
		if err != nil {
			return nil, err
		}
		d.Completed = completed == 1
		deposits = append(deposits, d)
	}

	return deposits, rows.Err()
}

// ListWithdrawals returns recent withdrawals for a user
func (db *Database) ListWithdrawals(userID string, limit int) ([]Withdrawal, error) {
	rows, err := db.inner.Query(
		"SELECT withdraw_id, user_id, timestamp, amount_raw, signature FROM withdrawals WHERE user_id = ? ORDER BY timestamp DESC LIMIT ?",
		userID, limit,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var withdrawals []Withdrawal
	for rows.Next() {
		var w Withdrawal
		err := rows.Scan(&w.WithdrawID, &w.UserID, &w.Timestamp, &w.AmountRaw, &w.Signature)
		if err != nil {
			return nil, err
		}
		withdrawals = append(withdrawals, w)
	}

	return withdrawals, rows.Err()
}

// AddRainChannel adds a channel to the rain whitelist for a server
func (db Database) AddRainChannel(serverID, channelID string) error {
	_, err := db.inner.Exec(
		"INSERT OR IGNORE INTO rain_channels (server_id, channel_id) VALUES (?, ?)",
		serverID, channelID,
	)
	return err
}

// RemoveRainChannel removes a channel from the rain whitelist
func (db Database) RemoveRainChannel(serverID, channelID string) error {
	_, err := db.inner.Exec(
		"DELETE FROM rain_channels WHERE server_id = ? AND channel_id = ?",
		serverID, channelID,
	)
	return err
}

// ClearRainChannels removes all rain channels for a server
func (db Database) ClearRainChannels(serverID string) error {
	_, err := db.inner.Exec(
		"DELETE FROM rain_channels WHERE server_id = ?",
		serverID,
	)
	return err
}

// GetRainChannels returns all whitelisted channels for a server
func (db Database) GetRainChannels(serverID string) ([]string, error) {
	rows, err := db.inner.Query(
		"SELECT channel_id FROM rain_channels WHERE server_id = ?",
		serverID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var channels []string
	for rows.Next() {
		var channelID string
		if err := rows.Scan(&channelID); err != nil {
			return nil, err
		}
		channels = append(channels, channelID)
	}
	return channels, rows.Err()
}

// IsRainChannel checks if a channel is whitelisted for rain
func (db Database) IsRainChannel(serverID, channelID string) (bool, error) {
	var count int
	err := db.inner.QueryRow(
		"SELECT COUNT(*) FROM rain_channels WHERE server_id = ? AND channel_id = ?",
		serverID, channelID,
	).Scan(&count)
	return count > 0, err
}
