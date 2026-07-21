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
