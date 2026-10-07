package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSanitizeFilename(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"nl-101.conf", "nl-101"},
		{"NL-102.CONF", "nl-102"},
		{"nl_201.conf", "nl-201"},
		{"special#1_node.conf", "special-1-node"},
		{"multiple---hyphens.conf", "multiple-hyphens"},
		{"-leading-trailing-.conf", "leading-trailing"},
	}

	for _, tc := range tests {
		got := sanitizeFilename(tc.input)
		if got != tc.expected {
			t.Errorf("sanitizeFilename(%q) = %q; want %q", tc.input, got, tc.expected)
		}
	}
}

func TestParseWireGuardConf_Success(t *testing.T) {
	tmpDir := t.TempDir()
	confPath := filepath.Join(tmpDir, "nl-101.conf")

	content := `
# WireGuard configuration for nl-101
[Interface]
PrivateKey = "AABBCCDDEEFF=="
Address = 10.2.0.2/32, 2a07:e01:1::2/128 # dual-stack
DNS = 10.2.0.1

[Peer]
PublicKey = 'GGHHIIJJKKLL=='
Endpoint = 185.159.157.2:51820
AllowedIPs = 0.0.0.0/0, ::/0
`
	if err := os.WriteFile(confPath, []byte(content), 0644); err != nil {
		t.Fatalf("failed to write test conf: %v", err)
	}

	node, err := parseWireGuardConf(confPath, "acc1")
	if err != nil {
		t.Fatalf("unexpected error parsing conf: %v", err)
	}

	if node.Tag != "proton-acc1-nl-101" {
		t.Errorf("Tag = %q; want 'proton-acc1-nl-101'", node.Tag)
	}
	if node.Server != "185.159.157.2" {
		t.Errorf("Server = %q; want '185.159.157.2'", node.Server)
	}
	if node.ServerPort != 51820 {
		t.Errorf("ServerPort = %d; want 51820", node.ServerPort)
	}
	if node.LocalAddress != "10.2.0.2/32" {
		t.Errorf("LocalAddress = %q; want '10.2.0.2/32'", node.LocalAddress)
	}
	if node.PrivateKey != "AABBCCDDEEFF==" {
		t.Errorf("PrivateKey = %q; want 'AABBCCDDEEFF=='", node.PrivateKey)
	}
	if node.PeerPublicKey != "GGHHIIJJKKLL==" {
		t.Errorf("PeerPublicKey = %q; want 'GGHHIIJJKKLL=='", node.PeerPublicKey)
	}
	if node.MTU != 1280 {
		t.Errorf("MTU = %d; want 1280 (default)", node.MTU)
	}
}

func TestParseWireGuardConf_CustomMTU(t *testing.T) {
	tmpDir := t.TempDir()
	confPath := filepath.Join(tmpDir, "nl-custom-mtu.conf")

	content := `
[Interface]
PrivateKey = privkey123=
Address = 10.2.0.2/32
MTU = 1360

[Peer]
PublicKey = pubkey123=
Endpoint = 185.159.157.3:51820
`
	if err := os.WriteFile(confPath, []byte(content), 0644); err != nil {
		t.Fatalf("failed to write test conf: %v", err)
	}

	node, err := parseWireGuardConf(confPath, "acc1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if node.MTU != 1360 {
		t.Errorf("MTU = %d; want 1360", node.MTU)
	}
}

func TestParseWireGuardConf_IPv6First(t *testing.T) {
	tmpDir := t.TempDir()
	confPath := filepath.Join(tmpDir, "nl-102.conf")

	content := `
[Interface]
PrivateKey = privkey123=
Address = 2a07:e01:1::2/128, 10.2.0.5/32

[Peer]
PublicKey = pubkey123=
Endpoint = 185.159.157.3:51820
`
	if err := os.WriteFile(confPath, []byte(content), 0644); err != nil {
		t.Fatalf("failed to write test conf: %v", err)
	}

	node, err := parseWireGuardConf(confPath, "acc1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if node.LocalAddress != "10.2.0.5/32" {
		t.Errorf("LocalAddress = %q; want '10.2.0.5/32'", node.LocalAddress)
	}
}

func TestParseWireGuardConf_ValidationErrors(t *testing.T) {
	tests := []struct {
		name    string
		content string
		wantErr string
	}{
		{
			name: "missing PrivateKey",
			content: `
[Interface]
Address = 10.2.0.2/32
[Peer]
PublicKey = pubkey=
Endpoint = 1.2.3.4:51820
`,
			wantErr: "missing required field 'PrivateKey'",
		},
		{
			name: "missing PublicKey",
			content: `
[Interface]
PrivateKey = privkey=
Address = 10.2.0.2/32
[Peer]
Endpoint = 1.2.3.4:51820
`,
			wantErr: "missing required field 'PublicKey'",
		},
		{
			name: "missing Endpoint",
			content: `
[Interface]
PrivateKey = privkey=
Address = 10.2.0.2/32
[Peer]
PublicKey = pubkey=
`,
			wantErr: "missing required field 'Endpoint'",
		},
		{
			name: "invalid Endpoint host:port",
			content: `
[Interface]
PrivateKey = privkey=
Address = 10.2.0.2/32
[Peer]
PublicKey = pubkey=
Endpoint = 1.2.3.4
`,
			wantErr: "invalid Endpoint",
		},
		{
			name: "invalid Endpoint port number",
			content: `
[Interface]
PrivateKey = privkey=
Address = 10.2.0.2/32
[Peer]
PublicKey = pubkey=
Endpoint = 1.2.3.4:99999
`,
			wantErr: "invalid port",
		},
		{
			name: "missing IPv4 Address",
			content: `
[Interface]
PrivateKey = privkey=
Address = 2a07:e01:1::2/128
[Peer]
PublicKey = pubkey=
Endpoint = 1.2.3.4:51820
`,
			wantErr: "missing or invalid IPv4 'Address'",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			tmpDir := t.TempDir()
			confPath := filepath.Join(tmpDir, "test.conf")
			if err := os.WriteFile(confPath, []byte(tc.content), 0644); err != nil {
				t.Fatalf("failed to write conf: %v", err)
			}
			_, err := parseWireGuardConf(confPath, "acc1")
			if err == nil {
				t.Fatalf("expected error containing %q, got nil", tc.wantErr)
			}
			if !strings.Contains(err.Error(), tc.wantErr) {
				t.Errorf("error = %q; want substring %q", err.Error(), tc.wantErr)
			}
		})
	}
}

func TestProcessConfigs_SkipEmptyDir(t *testing.T) {
	tmpDir := t.TempDir()

	acc1Dir := filepath.Join(tmpDir, "acc1")
	acc2EmptyDir := filepath.Join(tmpDir, "acc2_empty")
	_ = os.Mkdir(acc1Dir, 0755)
	_ = os.Mkdir(acc2EmptyDir, 0755)

	confContent := `
[Interface]
PrivateKey = priv1=
Address = 10.2.0.2/32
[Peer]
PublicKey = pub1=
Endpoint = 185.159.157.2:51820
`
	_ = os.WriteFile(filepath.Join(acc1Dir, "nl-101.conf"), []byte(confContent), 0644)

	cfg, err := processConfigs(tmpDir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(cfg.SingboxProtonAccounts) != 1 {
		t.Fatalf("len(cfg.SingboxProtonAccounts) = %d; want 1", len(cfg.SingboxProtonAccounts))
	}
	if cfg.SingboxProtonAccounts[0].Name != "acc1" {
		t.Errorf("Account name = %q; want 'acc1'", cfg.SingboxProtonAccounts[0].Name)
	}
}

func TestGenerateYAML_SchemaMatch(t *testing.T) {
	cfg := &Config{
		SingboxProtonAccounts: []Account{
			{
				Name:     "acc1",
				GroupTag: "proton-group-acc1",
				Nodes: []Node{
					{
						Tag:           "proton-acc1-nl-101",
						Server:        "185.159.157.2",
						ServerPort:    51820,
						LocalAddress:  "10.2.0.2/32",
						PrivateKey:    "PRIVATE_KEY_HERE",
						PeerPublicKey: "PUBLIC_KEY_HERE",
						MTU:           1280,
					},
				},
			},
		},
	}

	yamlBytes, err := generateYAML(cfg)
	if err != nil {
		t.Fatalf("generateYAML error: %v", err)
	}

	output := string(yamlBytes)
	expectedLines := []string{
		"---",
		"# Auto-generated by tools/proton-parser. DO NOT EDIT MANUALLY.",
		"singbox_proton_accounts:",
		"  - name: \"acc1\"",
		"    group_tag: \"proton-group-acc1\"",
		"    nodes:",
		"      - tag: \"proton-acc1-nl-101\"",
		"        server: \"185.159.157.2\"",
		"        server_port: 51820",
		"        local_address: \"10.2.0.2/32\"",
		"        private_key: \"PRIVATE_KEY_HERE\"",
		"        peer_public_key: \"PUBLIC_KEY_HERE\"",
		"        mtu: 1280",
	}

	for _, expected := range expectedLines {
		if !strings.Contains(output, expected) {
			t.Errorf("output does not contain line: %q\nFull output:\n%s", expected, output)
		}
	}
}

func TestEncryptFile(t *testing.T) {
	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "vault_test.yml")
	passFile := filepath.Join(tmpDir, "vault_pass.txt")

	if err := os.WriteFile(testFile, []byte("test_key: \"test_value\"\n"), 0600); err != nil {
		t.Fatalf("failed to write test file: %v", err)
	}
	if err := os.WriteFile(passFile, []byte("mock_password_12345\n"), 0600); err != nil {
		t.Fatalf("failed to write pass file: %v", err)
	}

	if err := encryptFile(testFile, passFile); err != nil {
		t.Fatalf("encryptFile failed: %v", err)
	}

	data, err := os.ReadFile(testFile)
	if err != nil {
		t.Fatalf("failed to read encrypted file: %v", err)
	}

	if !strings.HasPrefix(string(data), "$ANSIBLE_VAULT;") {
		t.Errorf("expected $ANSIBLE_VAULT; header, got:\n%s", string(data))
	}

	// Verify idempotency when already encrypted
	if err := encryptFile(testFile, passFile); err != nil {
		t.Errorf("expected encryptFile to succeed idempotently, got: %v", err)
	}
}

func TestIsEncryptedVault(t *testing.T) {
	tmpDir := t.TempDir()
	plainFile := filepath.Join(tmpDir, "plain.yml")
	vaultFile := filepath.Join(tmpDir, "vault.yml")

	if err := os.WriteFile(plainFile, []byte("plain_key: val\n"), 0600); err != nil {
		t.Fatalf("failed to write plain file: %v", err)
	}
	if err := os.WriteFile(vaultFile, []byte("$ANSIBLE_VAULT;1.1;AES256\n1234\n"), 0600); err != nil {
		t.Fatalf("failed to write vault file: %v", err)
	}

	isEncrypted, err := isEncryptedVault(plainFile)
	if err != nil || isEncrypted {
		t.Errorf("isEncryptedVault(plain) = %v, %v; want false, nil", isEncrypted, err)
	}

	isEncrypted, err = isEncryptedVault(vaultFile)
	if err != nil || !isEncrypted {
		t.Errorf("isEncryptedVault(vault) = %v, %v; want true, nil", isEncrypted, err)
	}
}

func TestParseVaultPassFromAnsibleCfg(t *testing.T) {
	tmpDir := t.TempDir()
	cfgPath := filepath.Join(tmpDir, "ansible.cfg")
	passPath := filepath.Join(tmpDir, ".custom_pass")

	if err := os.WriteFile(passPath, []byte("secret\n"), 0600); err != nil {
		t.Fatalf("failed to write pass file: %v", err)
	}

	cfgContent := `
[defaults]
inventory = hosts.ini
vault_password_file = .custom_pass
`
	if err := os.WriteFile(cfgPath, []byte(cfgContent), 0644); err != nil {
		t.Fatalf("failed to write cfg: %v", err)
	}

	got := parseVaultPassFromAnsibleCfg(cfgPath)
	if got != passPath {
		t.Errorf("parseVaultPassFromAnsibleCfg() = %q; want %q", got, passPath)
	}
}

func TestFindDefaultVaultPassFile_FromOutPath(t *testing.T) {
	tmpDir := t.TempDir()
	// Structure: tmpDir/ansible/.vault_pass and tmpDir/ansible/roles/singbox/vars/vault_proton.yml
	varsDir := filepath.Join(tmpDir, "ansible", "roles", "singbox", "vars")
	if err := os.MkdirAll(varsDir, 0755); err != nil {
		t.Fatalf("failed to create dirs: %v", err)
	}

	passPath := filepath.Join(tmpDir, "ansible", ".vault_pass")
	if err := os.WriteFile(passPath, []byte("pw\n"), 0600); err != nil {
		t.Fatalf("failed to write pass file: %v", err)
	}

	outPath := filepath.Join(varsDir, "vault_proton.yml")
	found := findDefaultVaultPassFile(outPath)
	if found != passPath {
		t.Errorf("findDefaultVaultPassFile(%q) = %q; want %q", outPath, found, passPath)
	}
}

