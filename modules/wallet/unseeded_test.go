package wallet

import (
	"testing"

	"github.com/rivine/rivine/types"
)

// TestIntegrationLoad1of1Siag loads a 1 of 1 unseeded key generated by siag
// and then tries to spend the siafunds contained within. The key is taken from
// the testing keys.
func TestIntegrationLoad1of1Siag(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}
	wt, err := createWalletTester(t.Name())
	if err != nil {
		t.Fatal(err)
	}
	defer wt.closeWt()

	// Load the key into the wallet.
	err = wt.wallet.LoadSiagKeys(wt.walletMasterKey, []string{"../../types/siag0of1of1.siakey"})
	if err != nil {
		t.Error(err)
	}

	// Create a second wallet that loads the persist structures of the existing
	// wallet. This wallet should have a siafund balance.
	w, err := New(wt.cs, wt.tpool, wt.wallet.persistDir)
	if err != nil {
		t.Fatal(err)
	}
	defer w.Close()
	err = w.Unlock(wt.walletMasterKey)
	if err != nil {
		t.Fatal(err)
	}
	_, siafundBal, _ := w.ConfirmedBalance()
	if !siafundBal.Equals64(2000) {
		t.Error("expecting a siafund balance of 2000 from the 1of1 key")
	}

	// Send some siafunds to the void.
	_, err = w.SendSiafunds(types.NewCurrency64(12), types.UnlockHash{})
	if err != nil {
		t.Fatal(err)
	}
	_, err = wt.miner.AddBlock()
	if err != nil {
		t.Fatal(err)
	}
	_, siafundBal, _ = w.ConfirmedBalance()
	if !siafundBal.Equals64(1988) {
		t.Error("expecting balance of 1988 after sending siafunds to the void")
	}
}

// TestIntegrationLoad2of3Siag loads a 2 of 3 unseeded key generated by siag
// and then tries to spend the siafunds contained within. The key is taken from
// the testing keys.
func TestIntegrationLoad2of3Siag(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}
	wt, err := createWalletTester(t.Name())
	if err != nil {
		t.Fatal(err)
	}
	defer wt.closeWt()

	// Load the key into the wallet.
	err = wt.wallet.LoadSiagKeys(wt.walletMasterKey, []string{"../../types/siag0of2of3.siakey", "../../types/siag1of2of3.siakey"})
	if err != nil {
		t.Error(err)
	}

	// Create a second wallet that loads the persist structures of the existing
	// wallet. This wallet should have a siafund balance.
	w, err := New(wt.cs, wt.tpool, wt.wallet.persistDir)
	if err != nil {
		t.Fatal(err)
	}
	defer w.Close()
	err = w.Unlock(wt.walletMasterKey)
	if err != nil {
		t.Fatal(err)
	}
	_, siafundBal, _ := w.ConfirmedBalance()
	if !siafundBal.Equals64(7000) {
		t.Error("expecting a siafund balance of 7000 from the 2of3 key")
	}

	// Send some siafunds to the void.
	_, err = w.SendSiafunds(types.NewCurrency64(12), types.UnlockHash{})
	if err != nil {
		t.Fatal(err)
	}
	_, err = wt.miner.AddBlock()
	if err != nil {
		t.Fatal(err)
	}
	_, siafundBal, _ = w.ConfirmedBalance()
	if !siafundBal.Equals64(6988) {
		t.Error("expecting balance of 6988 after sending siafunds to the void")
	}
}
