package util

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"
	"time"

	"github.com/gagliardetto/solana-go"
	"github.com/gagliardetto/solana-go/rpc"
	"github.com/ivypowered/ivy-sprite-bot/constants"
)

// Generate a 32-byte unique deposit/withdraw ID
func GenerateID(amountRaw uint64) [32]byte {
	var id [32]byte
	_, err := io.ReadFull(rand.Reader, id[:24])
	if err != nil {
		panic(err)
	}
	binary.LittleEndian.PutUint64(id[24:], amountRaw)
	return id
}

// Create a URL for a wallet link request
func LinkGenerateURL(wallet [32]byte, id string) string {
	return fmt.Sprintf(
		"https://sprite.ivypowered.com/link?wallet=%s&id=%s&timestamp=%d",
		solana.PublicKey(wallet).String(), id, time.Now().Unix(),
	)
}

// Verify the wallet linking, returning the user if successful
func LinkVerify(response [104]byte, id string) ([32]byte, error) {
	wallet := solana.PublicKey(response[0:32])
	signature := solana.Signature(response[32:96])
	timestamp := binary.LittleEndian.Uint64(response[96:])
	msg := fmt.Sprintf("Link wallet %s to ivy-sprite user %s at %d", wallet.String(), id, timestamp)
	if !signature.Verify(wallet, []byte(msg)) {
		return [32]byte{}, errors.New("invalid signature for wallet linking")
	}
	return wallet, nil
}

// Sign a withdrawal message using ed25519
func SignWithdrawal(vault [32]byte, user [32]byte, id [32]byte, privkey [64]byte) [64]byte {
	privateKey := ed25519.PrivateKey(privkey[:])

	// Create the message: vault_address (32 bytes) + user_key (32 bytes) + withdraw_id (32 bytes)
	message := make([]byte, 0, 96)
	message = append(message, vault[:]...)
	message = append(message, user[:]...)
	message = append(message, id[:]...)

	// Sign the message
	signature := ed25519.Sign(privateKey, message)
	var s [64]byte
	copy(s[:], signature[:])
	return s
}

const VAULT_PREFIX string = "vault"
const VAULT_DEPOSIT_PREFIX string = "vault_deposit"
const VAULT_WITHDRAW_PREFIX string = "vault_withdraw"

var IVY_PROGRAM_ID solana.PublicKey = solana.MustPublicKeyFromBase58("DkGdbW8SJmUoVE9KaBRwrvsQVhcuidy47DimjrhSoySE")

// Check whether a deposit is complete or not
func IsDepositComplete(r *rpc.Client, vault [32]byte, id [32]byte) (bool, error) {
	deposit, _, err := solana.FindProgramAddress([][]byte{
		[]byte(VAULT_DEPOSIT_PREFIX),
		vault[:],
		id[:],
	}, IVY_PROGRAM_ID)
	if err != nil {
		return false, err
	}
	info, err := r.GetAccountInfo(context.Background(), deposit)
	if err == rpc.ErrNotFound {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return info.Value != nil && info.Value.Lamports > 0, nil
}

// parse an amount string and convert it to IVY
func ParseAmount(amount string) (float64, error) {
	amount = strings.TrimSpace(amount)
	if len(amount) == 0 {
		return 0.0, nil
	}

	var x float64
	if amount[0] == '$' {
		usd, err := strconv.ParseFloat(amount[1:], 64)
		if err != nil {
			return 0.0, err
		}
		x = usd / constants.PRICE.Get(constants.RPC_CLIENT)
	} else {
		var err error
		x, err = strconv.ParseFloat(amount, 64)
		if err != nil {
			return 0.0, err
		}
	}
	if x < 0 {
		return 0.0, errors.New("no negative amounts allowed")
	}
	if x != x {
		return 0.0, errors.New("no NaNs allowed")
	}
	return x, nil
}
