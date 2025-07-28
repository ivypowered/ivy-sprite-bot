package constants

import (
	"encoding/hex"
	"os"
	"strconv"

	"github.com/gagliardetto/solana-go"
	"github.com/gagliardetto/solana-go/rpc"
	"github.com/ivypowered/ivy-sprite-bot/price"
)

func MustDecodeHexPrivateKey(k string) [64]byte {
	bytes, err := hex.DecodeString(k)
	if err != nil {
		panic("can't decode hex key: " + err.Error())
	}
	if len(bytes) != 64 {
		panic("can't decode hex key: required length 64, got " + strconv.Itoa(len(bytes)))
	}
	var b [64]byte
	copy(b[:], bytes[:])
	return b
}

var PRICE price.Price
var RPC_CLIENT *rpc.Client = rpc.New(os.Getenv("RPC_URL"))
var SPRITE_VAULT [32]byte = solana.MustPublicKeyFromBase58("AVXJfx8UsdkTPBL2UHuVDb3QVPvBw7P1sDH4fRXF1WiH")
var WITHDRAW_AUTHORITY_KEY [64]byte = MustDecodeHexPrivateKey(os.Getenv("WITHDRAW_AUTHORITY_KEY"))
var WITHDRAW_AUTHORITY [32]byte = solana.PrivateKey(WITHDRAW_AUTHORITY_KEY[:]).PublicKey()
var WITHDRAW_AUTHORITY_B58 string = solana.PublicKey(WITHDRAW_AUTHORITY).String()

const IVY_GREEN = 0x34D399
const IVY_RED = 0xFF5000
const IVY_PURPLE = 0x800080
const IVY_YELLOW = 0xFDC700
const IVY_WHITE = 0xFFFFFF

// Minimum rain amount
const RAIN_MIN_AMOUNT = 0.01

// Amount of people who have to be active for rain to work
const RAIN_MIN_ACTIVE_COUNT = 5

// Activity score required to receive rain
const RAIN_ACTIVITY_REQUIREMENT = 5

// Maximum activity score
const ACTIVITY_MAX = 10
