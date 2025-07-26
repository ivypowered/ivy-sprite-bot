package price

import (
	"context"
	"encoding/binary"
	"errors"
	"sync"
	"time"

	"github.com/gagliardetto/solana-go"
	"github.com/gagliardetto/solana-go/rpc"
)

var IVY_POOL = solana.MustPublicKeyFromBase58("2NuvyEVTus5PgrTzJcCKXdF1kJczBJmuPm41y5BZbpqC")
var USDC_POOL = solana.MustPublicKeyFromBase58("CyehsvWv3pVzbf7gSs98MUm3vTjfRjqixTQjRfFEwxnF")

type Price struct {
	mu          sync.Mutex
	price       float64
	lastUpdated uint64
	updating    bool
}

func getTokenBalance(a *rpc.Account) (uint64, error) {
	if a == nil || a.Data == nil {
		return 0, errors.New("nil value passed to getTokenBalance")
	}
	bytes := a.Data.GetBinary()
	if len(bytes) < 165 {
		return 0, errors.New("invalid token account length")
	}
	return binary.LittleEndian.Uint64(bytes[64:72]), nil
}

func (p *Price) Update(r *rpc.Client) error {
	res, err := r.GetMultipleAccounts(context.Background(), IVY_POOL, USDC_POOL)
	if err != nil {
		return err
	}
	if len(res.Value) != 2 {
		return errors.New("not enough accounts returned")
	}
	ivy_balance, err := getTokenBalance(res.Value[0])
	if err != nil {
		return err
	}
	usdc_balance, err := getTokenBalance(res.Value[1])
	if err != nil {
		return err
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	p.updating = false
	p.price = (float64(usdc_balance) / (1_000_000.0)) / (float64(ivy_balance) / (1_000_000_000.0))
	p.lastUpdated = uint64(time.Now().Unix())
	return nil
}

func saturatingSubU64(a, b uint64) uint64 {
	if a < b {
		return 0
	}
	return a - b
}

func (p *Price) Get(r *rpc.Client) float64 {
	p.mu.Lock()
	price := p.price
	timestamp := uint64(time.Now().Unix())
	delta := saturatingSubU64(timestamp, p.lastUpdated)
	if delta > 60 && !p.updating {
		// queue update
		p.updating = true
		p.mu.Unlock()
		go p.Update(r)
	} else {
		p.mu.Unlock()
	}
	return price
}
