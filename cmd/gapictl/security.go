package main

import (
	"encoding/hex"
	"fmt"
	"os"

	"github.com/goppydae/gapi/core/crypto"
	"github.com/spf13/cobra"
)

var signCmd = &cobra.Command{
	Use:   "sign [file] [private-key]",
	Short: "Sign a file and produce a detached .sig",
	Args:  cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		file := args[0]
		keyPath := args[1]

		key, err := crypto.LoadPrivate(keyPath)
		if err != nil {
			return fmt.Errorf("load key: %w", err)
		}

		// Hash file
		hash, err := crypto.HashFile(file)
		if err != nil {
			return fmt.Errorf("hash file: %w", err)
		}

		// Sign hash
		sig := key.Sign([]byte(hash))
		sigHex := hex.EncodeToString(sig)

		// Write .sig
		sigPath := file + ".sig"
		if err := os.WriteFile(sigPath, []byte(sigHex), 0644); err != nil {
			return err
		}

		fmt.Printf("Signed %s -> %s\n", file, sigPath)
		return nil
	},
}

var keygenCmd = &cobra.Command{
	Use:   "keygen [output-prefix]",
	Short: "Generate a new Ed25519 keypair",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		prefix := args[0]
		key, err := crypto.GenerateKey()
		if err != nil {
			return err
		}

		if err := key.SavePrivate(prefix + ".key"); err != nil {
			return err
		}
		if err := key.SavePublic(prefix + ".pub"); err != nil {
			return err
		}

		fmt.Printf("Generated %s.key and %s.pub\n", prefix, prefix)
		return nil
	},
}

var verifyCmd = &cobra.Command{
	Use:   "verify [file] [public-key]",
	Short: "Verify a file against its .sig",
	Args:  cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		file := args[0]
		keyPath := args[1]

		pub, err := crypto.LoadPublic(keyPath)
		if err != nil {
			return fmt.Errorf("load pub key: %w", err)
		}

		// Read signature
		sigHexBytes, err := os.ReadFile(file + ".sig")
		if err != nil {
			return fmt.Errorf("read signature: %w", err)
		}
		sig, err := hex.DecodeString(string(sigHexBytes))
		if err != nil {
			return fmt.Errorf("decode signature: %w", err)
		}

		// Hash file
		hash, err := crypto.HashFile(file)
		if err != nil {
			return fmt.Errorf("hash file: %w", err)
		}

		valid := crypto.Verify(pub, []byte(hash), sig)
		if valid {
			fmt.Println("OK")
			return nil
		}
		return fmt.Errorf("signature verification FAILED")
	},
}

func init() {
	rootCmd.AddCommand(signCmd)
	rootCmd.AddCommand(keygenCmd)
	rootCmd.AddCommand(verifyCmd)
}
