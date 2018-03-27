package client

import (
	"encoding/base64"
	"fmt"
	"math/big"
	"os"
	"text/tabwriter"

	"github.com/rivine/rivine/modules"

	"github.com/bgentry/speakeasy"
	"github.com/spf13/cobra"

	"github.com/rivine/rivine/api"
	"github.com/rivine/rivine/types"
)

var (
	walletCmd = &cobra.Command{
		Use:   "wallet",
		Short: "Perform wallet actions",
		Long: `Generate a new address, send coins to another wallet, or view info about the wallet.

Units:
The following units are supported:
  p (pico,  10^-12)
  n (nano,  10^-9 )
  u (micro, 10^-6 )
  m (milli, 10^-3 )
  C
  K (kilo,  10^3  )
  M (mega,  10^6  )
  G (giga,  10^9  )
  T (tera,  10^12 )`,
		Run: wrap(Walletbalancecmd),
	}

	walletBlockStakeStatCmd = &cobra.Command{
		Use:   "blockstakestat",
		Short: "Get the stats of the blockstake",
		Long:  "Gives all the statistical info of the blockstake.",
		Run:   wrap(Walletblockstakestatcmd),
	}

	walletAddressCmd = &cobra.Command{
		Use:   "address",
		Short: "Get a new wallet address",
		Long:  "Generate a new wallet address from the wallet's primary seed.",
		Run:   wrap(Walletaddresscmd),
	}

	walletAddressesCmd = &cobra.Command{
		Use:   "addresses",
		Short: "List all addresses",
		Long:  "List all addresses that have been generated by the wallet",
		Run:   wrap(Walletaddressescmd),
	}

	walletInitCmd = &cobra.Command{
		Use:   "init",
		Short: "Initialize and encrypt a new wallet",
		Long: `Generate a new wallet from a randomly generated seed, and encrypt it.
By default the wallet encryption / unlock password is the same as the generated seed.`,
		Run: wrap(Walletinitcmd),
	}

	walletLoadCmd = &cobra.Command{
		Use:   "load",
		Short: "Load a wallet seed",
		// Run field is not set, as the load command itself is not a valid command.
		// A subcommand must be provided.
	}

	walletLoadSeedCmd = &cobra.Command{
		Use:   `seed`,
		Short: "Add a seed to the wallet",
		Long:  "Uses the given password to create a new wallet with that as the primary seed",
		Run:   wrap(Walletloadseedcmd),
	}

	walletLockCmd = &cobra.Command{
		Use:   "lock",
		Short: "Lock the wallet",
		Long:  "Lock the wallet, preventing further use",
		Run:   wrap(Walletlockcmd),
	}

	walletSeedsCmd = &cobra.Command{
		Use:   "seeds",
		Short: "Retrieve information about your seeds",
		Long:  "Retrieves the current seed, how many addresses are remaining, and the rest of your seeds from the wallet",
		Run:   wrap(Walletseedscmd),
	}

	walletSendCmd = &cobra.Command{
		Use:   "send",
		Short: "Send either coins or blockstakes to an address",
		Long:  "Send either coins or blockstakes to an address",
		// Run field is not set, as the load command itself is not a valid command.
		// A subcommand must be provided.
	}

	walletSendSiacoinsCmd = &cobra.Command{
		Use:   "coins [amount] [dest]",
		Short: "Send coins to an address",
		Long: `Send coins to an address. 'dest' must be a 76-byte hexadecimal address.
'amount' can be specified in units, e.g. 1.23K. Run 'wallet --help' for a list of units.
A unit must be supplied

A miner fee of 10 C is levied on all transactions.`,
		Run: wrap(Walletsendsiacoinscmd),
	}

	walletSendSiafundsCmd = &cobra.Command{
		Use:   "blockstakes [amount] [dest]",
		Short: "Send blockstakes",
		Long: `Send blockstakes to an address.
Run 'wallet send --help' to see a list of available units.`,
		Run: wrap(Walletsendblockstakescmd),
	}

	walletRegisterDataCmd = &cobra.Command{
		Use:   "registerdata [namespace] [data] [dest]",
		Short: "Register data on the blockchain",
		Long:  "Register data on the blockcahin by sending a minimal transaction to the destination address, and including the data in the transaction",
		Run:   wrap(Walletregisterdatacmd),
	}

	walletBalanceCmd = &cobra.Command{
		Use:   "balance",
		Short: "View wallet balance",
		Long:  "View wallet balance, including confirmed and unconfirmed coins and blockstakes.",
		Run:   wrap(Walletbalancecmd),
	}

	walletTransactionsCmd = &cobra.Command{
		Use:   "transactions",
		Short: "View transactions",
		Long:  "View transactions related to addresses spendable by the wallet, providing a net flow of coins and blockstakes for each transaction",
		Run:   wrap(Wallettransactionscmd),
	}

	walletUnlockCmd = &cobra.Command{
		Use:   `unlock`,
		Short: "Unlock the wallet",
		Long:  "Decrypt and load the wallet into memory",
		Run:   wrap(Walletunlockcmd),
	}
)

// Walletaddresscmd fetches a new address from the wallet that will be able to
// receive coins.
func Walletaddresscmd() {
	addr := new(api.WalletAddressGET)
	err := GetAPI("/wallet/address", addr)
	if err != nil {
		Die("Could not generate new address:", err)
	}
	fmt.Printf("Created new address: %s\n", addr.Address)
}

// Walletaddressescmd fetches the list of addresses that the wallet knows.
func Walletaddressescmd() {
	addrs := new(api.WalletAddressesGET)
	err := GetAPI("/wallet/addresses", addrs)
	if err != nil {
		Die("Failed to fetch addresses:", err)
	}
	for _, addr := range addrs.Addresses {
		fmt.Println(addr)
	}
}

// Walletinitcmd encrypts the wallet with the given password
func Walletinitcmd() {
	var er api.WalletInitPOST

	fmt.Println("You should provide a passphrase, it may be empty if you wish.")

	passphrase, err := speakeasy.Ask("Wallet passphrase: ")
	if err != nil {
		Die("Reading passphrase failed:", err)
	}
	if passphrase == "" {
		Die("passphrase is required and cannot be empty")
	}

	repassphrase, err := speakeasy.Ask("Reenter passphrase: ")
	if err != nil {
		Die("Reading passphrase failed:", err)
	}

	if repassphrase != passphrase {
		Die("Given passphrases do not match !!")
	}

	qs := fmt.Sprintf("passphrase=%s", passphrase)

	err = PostResp("/wallet/init", qs, &er)
	if err != nil {
		Die("Error when encrypting wallet:", err)
	}

	fmt.Printf("Recovery seed:\n%s\n\n", er.PrimarySeed)
	fmt.Printf("Wallet encrypted with given passphrase\n")
}

// Walletloadseedcmd adds a seed to the wallet's list of seeds
func Walletloadseedcmd() {
	passphrase, err := speakeasy.Ask("Wallet passphrase: ")
	if err != nil {
		Die("Reading passphrase failed:", err)
	}
	mnemonic, err := speakeasy.Ask("New Mnemonic: ")
	if err != nil {
		Die("Reading seed failed:", err)
	}
	qs := fmt.Sprintf("passphrase=%s&mnemonic=%s", passphrase, mnemonic)
	err = Post("/wallet/seed", qs)
	if err != nil {
		Die("Could not add seed:", err)
	}
	fmt.Println("Added Key")
}

// Walletlockcmd locks the wallet
func Walletlockcmd() {
	err := Post("/wallet/lock", "")
	if err != nil {
		Die("Could not lock wallet:", err)
	}
}

// Walletseedscmd returns the current seed {
func Walletseedscmd() {
	var seedInfo api.WalletSeedsGET
	err := GetAPI("/wallet/seeds", &seedInfo)
	if err != nil {
		Die("Error retrieving the current seed:", err)
	}
	fmt.Printf("Primary Seed: %s\n"+
		"Addresses Remaining %d\n"+
		"All Seeds:\n", seedInfo.PrimarySeed, seedInfo.AddressesRemaining)
	for _, seed := range seedInfo.AllSeeds {
		fmt.Println(seed)
	}
}

// Walletsendsiacoinscmd sends siacoins to a destination address.
func Walletsendsiacoinscmd(amount, dest string) {
	hastings, err := ParseCurrency(amount)
	if err != nil {
		Die("Could not parse amount:", err)
	}
	err = Post("/wallet/coins", fmt.Sprintf("amount=%s&destination=%s", hastings, dest))
	if err != nil {
		Die("Could not send coins:", err)
	}
	fmt.Printf("Sent %s hastings to %s\n", hastings, dest)
}

// Walletsendblockstakescmd sends siafunds to a destination address.
func Walletsendblockstakescmd(amount, dest string) {
	err := Post("/wallet/blockstakes", fmt.Sprintf("amount=%s&destination=%s", amount, dest))
	if err != nil {
		Die("Could not send blockstakes:", err)
	}
	fmt.Printf("Sent %s blockstakes to %s\n", amount, dest)
}

// Walletregisterdatacmd registers data on the blockchain by making a minimal transaction to the designated address
// and includes the data in the transaction
func Walletregisterdatacmd(namespace, data, dest string) {
	// At the moment, we need to prepend the non sia prefix for the transaction to be accepted by the transactionpool
	dataBytes := append(modules.PrefixNonSia[:], []byte(namespace)...)
	dataBytes = append(dataBytes, []byte(data)...)
	encodedData := base64.StdEncoding.EncodeToString(dataBytes)
	err := Post("/wallet/data", fmt.Sprintf("destination=%s&data=%s", dest, encodedData))
	if err != nil {
		Die("Could not register data:", err)
	}
	fmt.Printf("Registered data to %s\n", dest)
}

// Walletblockstakestatcmd gives all statistical info of blockstake
func Walletblockstakestatcmd() {
	bsstat := new(api.WalletBlockStakeStatsGET)
	err := GetAPI("/wallet/blockstakestats", bsstat)
	if err != nil {
		Die("Could not gen blockstake info:", err)
	}
	fmt.Printf("BlockStake stats:\n")
	fmt.Printf("Total active Blockstake is %v\n", bsstat.TotalActiveBlockStake)
	fmt.Printf("This account has %v Blockstake\n", bsstat.TotalBlockStake)
	fmt.Printf("%v of last %v Blocks created (theoretically %v)\n", bsstat.TotalBCLast1000, bsstat.BlockCount, bsstat.TotalBCLast1000t)
	fmt.Printf("containing %v fee \n", CurrencyUnits(bsstat.TotalFeeLast1000))

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', tabwriter.AlignRight|tabwriter.Debug)
	fmt.Fprintln(w, "state\t#BlockStake\tUTXO hash\t")

	for i, BSstate := range bsstat.BlockStakeState {
		state := "active"
		if BSstate == 0 {
			state = "not active"
		}
		fmt.Fprintf(w, "%v\t%v\t%v\t\n", state, bsstat.BlockStakeNumOf[i], bsstat.BlockStakeUTXOAddress[i])
	}
	w.Flush()
}

// Walletbalancecmd retrieves and displays information about the wallet.
func Walletbalancecmd() {
	status := new(api.WalletGET)
	err := GetAPI("/wallet", status)
	if err != nil {
		Die("Could not get wallet status:", err)
	}
	encStatus := "Unencrypted"
	if status.Encrypted {
		encStatus = "Encrypted"
	}
	if !status.Unlocked {
		fmt.Printf(`Wallet status:
%v, Locked
Unlock the wallet to view balance
`, encStatus)
		return
	}

	unconfirmedBalance := status.ConfirmedCoinBalance.Add(status.UnconfirmedIncomingCoins).Sub(status.UnconfirmedOutgoingCoins)
	var delta string
	if unconfirmedBalance.Cmp(status.ConfirmedCoinBalance) >= 0 {
		delta = "+" + CurrencyUnits(unconfirmedBalance.Sub(status.ConfirmedCoinBalance))
	} else {
		delta = "-" + CurrencyUnits(status.ConfirmedCoinBalance.Sub(unconfirmedBalance))
	}

	fmt.Printf(`Wallet status:
%s, Unlocked
Confirmed Balance:   %v
Unconfirmed Delta:   %v
Exact:               %v H
BlockStakes:         %v BS
`, encStatus, CurrencyUnits(status.ConfirmedCoinBalance), delta,
		status.ConfirmedCoinBalance, status.BlockStakeBalance)
}

// Wallettransactionscmd lists all of the transactions related to the wallet,
// providing a net flow of siacoins and siafunds for each.
func Wallettransactionscmd() {
	wtg := new(api.WalletTransactionsGET)
	err := GetAPI("/wallet/transactions?startheight=0&endheight=10000000", wtg)
	if err != nil {
		Die("Could not fetch transaction history:", err)
	}

	fmt.Println("    [height]                                                   [transaction id]       [net coins]   [net blockstakes]")
	txns := append(wtg.ConfirmedTransactions, wtg.UnconfirmedTransactions...)
	for _, txn := range txns {
		// Determine the number of outgoing siacoins and siafunds.
		var outgoingSiacoins types.Currency
		var outgoingBlockStakes types.Currency
		for _, input := range txn.Inputs {
			if input.FundType == types.SpecifierCoinInput && input.WalletAddress {
				outgoingSiacoins = outgoingSiacoins.Add(input.Value)
			}
			if input.FundType == types.SpecifierBlockStakeInput && input.WalletAddress {
				outgoingBlockStakes = outgoingBlockStakes.Add(input.Value)
			}
		}

		// Determine the number of incoming siacoins and siafunds.
		var incomingSiacoins types.Currency
		var incomingBlockStakes types.Currency
		for _, output := range txn.Outputs {
			if output.FundType == types.SpecifierMinerPayout {
				incomingSiacoins = incomingSiacoins.Add(output.Value)
			}
			if output.FundType == types.SpecifierCoinOutput && output.WalletAddress {
				incomingSiacoins = incomingSiacoins.Add(output.Value)
			}
			if output.FundType == types.SpecifierBlockStakeOutput && output.WalletAddress {
				incomingBlockStakes = incomingBlockStakes.Add(output.Value)
			}
		}

		// Convert the siacoins to a float.
		incomingSiacoinsFloat, _ := new(big.Rat).SetFrac(incomingSiacoins.Big(), types.OneCoin.Big()).Float64()
		outgoingSiacoinsFloat, _ := new(big.Rat).SetFrac(outgoingSiacoins.Big(), types.OneCoin.Big()).Float64()

		// Print the results.
		if txn.ConfirmationHeight < 1e9 {
			fmt.Printf("%12v", txn.ConfirmationHeight)
		} else {
			fmt.Printf(" unconfirmed")
		}
		fmt.Printf("%67v%15.2f C", txn.TransactionID, incomingSiacoinsFloat-outgoingSiacoinsFloat)
		incomingBlockStakeBigInt := incomingBlockStakes.Big()
		outgoingBlockStakeBigInt := outgoingBlockStakes.Big()
		fmt.Printf("%14s BS\n", new(big.Int).Sub(incomingBlockStakeBigInt, outgoingBlockStakeBigInt).String())
	}
}

// Walletunlockcmd unlocks a saved wallet
func Walletunlockcmd() {
	password, err := speakeasy.Ask("Wallet password: ")
	if err != nil {
		Die("Reading password failed:", err)
	}
	fmt.Println("Unlocking the wallet. This may take several minutes...")
	qs := fmt.Sprintf("passphrase=%s", password)
	err = Post("/wallet/unlock", qs)
	if err != nil {
		Die("Could not unlock wallet:", err)
	}
	fmt.Println("Wallet unlocked")
}
