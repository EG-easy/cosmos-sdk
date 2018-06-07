package keeper

import (
	"testing"

	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/cosmos/cosmos-sdk/x/stake/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGetInflation(t *testing.T) {
	ctx, _, keeper := CreateTestInput(t, false, 0)
	pool := keeper.GetPool(ctx)
	params := keeper.GetParams(ctx)
	hrsPerYrRat := sdk.NewRat(hrsPerYr)

	// Governing Mechanism:
	//    BondedRatio = BondedTokens / TotalSupply
	//    inflationRateChangePerYear = (1- BondedRatio/ GoalBonded) * MaxInflationRateChange

	tests := []struct {
		name                            string
		setBondedTokens, setLooseTokens int64
		setInflation, expectedChange    sdk.Rat
	}{
		// with 0% bonded atom supply the inflation should increase by InflationRateChange
		{"test 1", 0, 0, sdk.NewRat(7, 100), params.InflationRateChange.Quo(hrsPerYrRat).Round(precision)},

		// 100% bonded, starting at 20% inflation and being reduced
		// (1 - (1/0.67))*(0.13/8667)
		{"test 2", 1, 0, sdk.NewRat(20, 100),
			sdk.OneRat().Sub(sdk.OneRat().Quo(params.GoalBonded)).Mul(params.InflationRateChange).Quo(hrsPerYrRat).Round(precision)},

		// 50% bonded, starting at 10% inflation and being increased
		{"test 3", 1, 1, sdk.NewRat(10, 100),
			sdk.OneRat().Sub(sdk.NewRat(1, 2).Quo(params.GoalBonded)).Mul(params.InflationRateChange).Quo(hrsPerYrRat).Round(precision)},

		// test 7% minimum stop (testing with 100% bonded)
		{"test 4", 1, 0, sdk.NewRat(7, 100), sdk.ZeroRat()},
		{"test 5", 1, 0, sdk.NewRat(70001, 1000000), sdk.NewRat(-1, 1000000).Round(precision)},

		// test 20% maximum stop (testing with 0% bonded)
		{"test 6", 0, 0, sdk.NewRat(20, 100), sdk.ZeroRat()},
		{"test 7", 0, 0, sdk.NewRat(199999, 1000000), sdk.NewRat(1, 1000000).Round(precision)},

		// perfect balance shouldn't change inflation
		{"test 8", 67, 33, sdk.NewRat(15, 100), sdk.ZeroRat()},
	}
	for _, tc := range tests {
		pool.BondedTokens, pool.LooseUnbondedTokens = tc.setBondedTokens, tc.setLooseTokens
		pool.Inflation = tc.setInflation
		keeper.SetPool(ctx, pool)

		inflation := keeper.NextInflation(ctx)
		diffInflation := inflation.Sub(tc.setInflation)

		assert.True(t, diffInflation.Equal(tc.expectedChange),
			"Name: %v\nDiff:  %v\nExpected: %v\n", tc.name, diffInflation, tc.expectedChange)
	}
}

func TestProcessProvisions(t *testing.T) {
	ctx, _, keeper := CreateTestInput(t, false, 0)
	params := types.DefaultParams()
	params.MaxValidators = 2
	keeper.SetParams(ctx, params)
	pool := keeper.GetPool(ctx)

	var tokenSupply int64 = 550000000
	var bondedShares int64 = 150000000
	var unbondedShares int64 = 400000000

	// create some validators some bonded, some unbonded
	var validators [5]types.Validator
	validators[0] = types.NewValidator(Addrs[0], PKs[0], types.Description{})
	validators[0], pool, _ = validators[0].AddTokensFromDel(pool, 150000000)
	keeper.SetPool(ctx, pool)
	validators[0] = keeper.UpdateValidator(ctx, validators[0])
	pool = keeper.GetPool(ctx)
	require.Equal(t, bondedShares, pool.BondedTokens, "%v", pool)

	validators[1] = types.NewValidator(Addrs[1], PKs[1], types.Description{})
	validators[1], pool, _ = validators[1].AddTokensFromDel(pool, 100000000)
	keeper.SetPool(ctx, pool)
	validators[1] = keeper.UpdateValidator(ctx, validators[1])
	validators[2] = types.NewValidator(Addrs[2], PKs[2], types.Description{})
	validators[2], pool, _ = validators[2].AddTokensFromDel(pool, 100000000)
	keeper.SetPool(ctx, pool)
	validators[2] = keeper.UpdateValidator(ctx, validators[2])
	validators[3] = types.NewValidator(Addrs[3], PKs[3], types.Description{})
	validators[3], pool, _ = validators[3].AddTokensFromDel(pool, 100000000)
	keeper.SetPool(ctx, pool)
	validators[3] = keeper.UpdateValidator(ctx, validators[3])
	validators[4] = types.NewValidator(Addrs[4], PKs[4], types.Description{})
	validators[4], pool, _ = validators[4].AddTokensFromDel(pool, 100000000)
	keeper.SetPool(ctx, pool)
	validators[4] = keeper.UpdateValidator(ctx, validators[4])

	assert.Equal(t, tokenSupply, pool.TokenSupply())
	assert.Equal(t, bondedShares, pool.BondedTokens)
	assert.Equal(t, unbondedShares, pool.UnbondedTokens)

	// initial bonded ratio ~ 27%
	assert.True(t, pool.BondedRatio().Equal(sdk.NewRat(bondedShares, tokenSupply)), "%v", pool.BondedRatio())

	// test the value of validator shares
	assert.True(t, pool.BondedShareExRate().Equal(sdk.OneRat()), "%v", pool.BondedShareExRate())

	initialSupply := pool.TokenSupply()
	initialUnbonded := pool.TokenSupply() - pool.BondedTokens

	// process the provisions a year
	for hr := 0; hr < 8766; hr++ {
		pool := keeper.GetPool(ctx)
		expInflation := keeper.NextInflation(ctx).Round(1000000000)
		expProvisions := (expInflation.Mul(sdk.NewRat(pool.TokenSupply())).Quo(hrsPerYrRat)).Evaluate()
		startBondedTokens := pool.BondedTokens
		startTotalSupply := pool.TokenSupply()
		pool = keeper.ProcessProvisions(ctx)
		keeper.SetPool(ctx, pool)
		//fmt.Printf("hr %v, startBondedTokens %v, expProvisions %v, pool.BondedTokens %v\n", hr, startBondedTokens, expProvisions, pool.BondedTokens)
		require.Equal(t, startBondedTokens+expProvisions, pool.BondedTokens, "hr %v", hr)
		require.Equal(t, startTotalSupply+expProvisions, pool.TokenSupply())
	}
	pool = keeper.GetPool(ctx)
	assert.NotEqual(t, initialSupply, pool.TokenSupply())
	assert.Equal(t, initialUnbonded, pool.UnbondedTokens)
	//panic(fmt.Sprintf("debug total %v, bonded  %v, diff %v\n", p.TotalSupply, p.BondedTokens, pool.TokenSupply()-pool.BondedTokens))

	// initial bonded ratio ~ from 27% to 40% increase for bonded holders ownership of total supply
	assert.True(t, pool.BondedRatio().Equal(sdk.NewRat(211813022, 611813022)), "%v", pool.BondedRatio())

	// global supply
	assert.Equal(t, int64(611813022), pool.TokenSupply())
	assert.Equal(t, int64(211813022), pool.BondedTokens)
	assert.Equal(t, unbondedShares, pool.UnbondedTokens)

	// test the value of validator shares
	assert.True(t, pool.BondedShareExRate().Mul(sdk.NewRat(bondedShares)).Equal(sdk.NewRat(211813022)), "%v", pool.BondedShareExRate())
}