package auth

import "testing"

func TestWalletFromPrivateKey(t *testing.T) {
	cases := []struct {
		name string
		key  string
		want string
	}{
		// Well-known vector: key 0x...01 -> address of generator point pubkey.
		{"one with 0x", "0x0000000000000000000000000000000000000000000000000000000000000001", "0x7E5F4552091A69125d5DfCb7b8C2659029395Bdf"},
		{"one without 0x", "0000000000000000000000000000000000000000000000000000000000000001", "0x7E5F4552091A69125d5DfCb7b8C2659029395Bdf"},
		// Hardhat/Anvil default account #0.
		{"anvil zero", "0xac0974bec39a17e36ba4a6b4d238ff944bacb478cbed5efcae784d7bf4f2ff80", "0xf39Fd6e51aad88F6F4ce6aB8827279cffFb92266"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, err := WalletFromPrivateKey(c.key)
			if err != nil {
				t.Fatalf("WalletFromPrivateKey(%q): %v", c.key, err)
			}
			if got != c.want {
				t.Fatalf("WalletFromPrivateKey(%q) = %s, want %s", c.key, got, c.want)
			}
		})
	}
}

func TestWalletFromPrivateKeyInvalid(t *testing.T) {
	for _, bad := range []string{"", "0x1234", "not-hex", "0x" + "zz" + "00"} {
		if _, err := WalletFromPrivateKey(bad); err == nil {
			t.Fatalf("WalletFromPrivateKey(%q): expected error, got nil", bad)
		}
	}
}

func TestSignMessage(t *testing.T) {
	// Anvil account #0; recovery cross-checked against eth_account.
	sig, err := SignMessage("0xac0974bec39a17e36ba4a6b4d238ff944bacb478cbed5efcae784d7bf4f2ff80", "Create an AIP agent")
	if err != nil {
		t.Fatal(err)
	}
	if len(sig) != 2+65*2 {
		t.Fatalf("signature length = %d, want %d", len(sig), 2+65*2)
	}
	if sig[:2] != "0x" {
		t.Fatalf("signature missing 0x prefix: %s", sig[:4])
	}
	v := sig[len(sig)-2:]
	if v != "1b" && v != "1c" {
		t.Fatalf("recovery byte v = %s, want 1b or 1c", v)
	}
}
