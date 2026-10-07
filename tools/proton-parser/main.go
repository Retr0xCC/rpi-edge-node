package main

import (
	"bufio"
	"bytes"
	"flag"
	"fmt"
	"log"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"
)

// Node represents a WireGuard VPN endpoint node configuration.
type Node struct {
	Tag           string `yaml:"tag"`
	Server        string `yaml:"server"`
	ServerPort    int    `yaml:"server_port"`
	LocalAddress  string `yaml:"local_address"`
	PrivateKey    string `yaml:"private_key"`
	PeerPublicKey string `yaml:"peer_public_key"`
	MTU           int    `yaml:"mtu,omitempty"`
}

// Account represents a Proton account containing multiple VPN nodes.
type Account struct {
	Name     string `yaml:"name"`
	GroupTag string `yaml:"group_tag"`
	Nodes    []Node `yaml:"nodes"`
}

// Config represents the root structure of the generated Ansible vault_proton.yml.
type Config struct {
	SingboxProtonAccounts []Account `yaml:"singbox_proton_accounts"`
}

// sanitizeFilename strips extension, converts to lowercase, and replaces non-alphanumeric chars with hyphens.
func sanitizeFilename(filename string) string {
	base := strings.TrimSuffix(filename, filepath.Ext(filename))
	base = strings.ToLower(base)
	var b strings.Builder
	for _, r := range base {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '-' {
			b.WriteRune(r)
		} else {
			b.WriteRune('-')
		}
	}
	res := b.String()
	for strings.Contains(res, "--") {
		res = strings.ReplaceAll(res, "--", "-")
	}
	return strings.Trim(res, "-")
}

// stripQuotes removes surrounding single or double quotes and trims whitespace.
func stripQuotes(val string) string {
	val = strings.TrimSpace(val)
	if len(val) >= 2 {
		if (val[0] == '"' && val[len(val)-1] == '"') || (val[0] == '\'' && val[len(val)-1] == '\'') {
			return strings.TrimSpace(val[1 : len(val)-1])
		}
	}
	return val
}

// parseWireGuardConf parses a single WireGuard .conf file and extracts required fields.
func parseWireGuardConf(filePath, accountName string) (Node, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return Node{}, fmt.Errorf("failed to open file %q: %w", filePath, err)
	}
	defer file.Close()

	var (
		currentSection string
		privateKey     string
		publicKey      string
		endpoint       string
		addressList    []string
		mtuStr         string
	)

	scanner := bufio.NewScanner(file)
	lineNum := 0
	for scanner.Scan() {
		lineNum++
		line := strings.TrimSpace(scanner.Text())

		// Skip comments and empty lines
		if line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, ";") {
			continue
		}

		// Section header
		if strings.HasPrefix(line, "[") && strings.HasSuffix(line, "]") {
			currentSection = strings.ToLower(strings.TrimSpace(line[1 : len(line)-1]))
			continue
		}

		// Key-Value pair
		var key, val string
		eqIdx := strings.Index(line, "=")
		colonIdx := strings.Index(line, ":")
		if eqIdx != -1 && (colonIdx == -1 || eqIdx < colonIdx) {
			key = strings.ToLower(strings.TrimSpace(line[:eqIdx]))
			val = strings.TrimSpace(line[eqIdx+1:])
		} else if colonIdx != -1 {
			key = strings.ToLower(strings.TrimSpace(line[:colonIdx]))
			val = strings.TrimSpace(line[colonIdx+1:])
		} else {
			continue
		}

		// Strip inline comments if any
		if cIdx := strings.IndexAny(val, "#;"); cIdx != -1 {
			val = strings.TrimSpace(val[:cIdx])
		}
		val = stripQuotes(val)

		switch currentSection {
		case "interface":
			switch key {
			case "privatekey":
				privateKey = val
			case "address":
				addressList = append(addressList, val)
			case "mtu":
				mtuStr = val
			}
		case "peer":
			switch key {
			case "publickey":
				publicKey = val
			case "endpoint":
				endpoint = val
			}
		}
	}

	if err := scanner.Err(); err != nil {
		return Node{}, fmt.Errorf("error reading file %q: %w", filePath, err)
	}

	// Validate PrivateKey
	if privateKey == "" {
		return Node{}, fmt.Errorf("file %q: missing required field 'PrivateKey' in [Interface]", filePath)
	}

	// Validate PublicKey
	if publicKey == "" {
		return Node{}, fmt.Errorf("file %q: missing required field 'PublicKey' in [Peer]", filePath)
	}

	// Validate Endpoint
	if endpoint == "" {
		return Node{}, fmt.Errorf("file %q: missing required field 'Endpoint' in [Peer]", filePath)
	}

	host, portStr, err := net.SplitHostPort(endpoint)
	if err != nil {
		return Node{}, fmt.Errorf("file %q: invalid Endpoint %q (expected host:port): %w", filePath, endpoint, err)
	}
	port, err := strconv.Atoi(portStr)
	if err != nil || port <= 0 || port > 65535 {
		return Node{}, fmt.Errorf("file %q: invalid port %q in Endpoint (must be 1-65535)", filePath, portStr)
	}

	// Extract first IPv4 Address retaining CIDR
	var localAddress string
	for _, rawAddr := range addressList {
		candidates := strings.Split(rawAddr, ",")
		for _, cand := range candidates {
			cand = stripQuotes(cand)
			if cand == "" {
				continue
			}
			if strings.Contains(cand, "/") {
				ip, _, err := net.ParseCIDR(cand)
				if err == nil && ip.To4() != nil {
					localAddress = cand
					break
				}
			} else {
				ip := net.ParseIP(cand)
				if ip != nil && ip.To4() != nil {
					localAddress = cand + "/32"
					break
				}
			}
		}
		if localAddress != "" {
			break
		}
	}

	if localAddress == "" {
		return Node{}, fmt.Errorf("file %q: missing or invalid IPv4 'Address' in [Interface]", filePath)
	}

	filenameSanitized := sanitizeFilename(filepath.Base(filePath))
	tag := fmt.Sprintf("proton-%s-%s", accountName, filenameSanitized)

	nodeMTU := 1280
	if mtuStr != "" {
		if parsed, err := strconv.Atoi(mtuStr); err == nil && parsed > 0 {
			nodeMTU = parsed
		}
	}

	return Node{
		Tag:           tag,
		Server:        host,
		ServerPort:    port,
		LocalAddress:  localAddress,
		PrivateKey:    privateKey,
		PeerPublicKey: publicKey,
		MTU:           nodeMTU,
	}, nil
}

// setDoubleQuotes ensures string scalar values in YAML mappings and sequences are double quoted.
func setDoubleQuotes(n *yaml.Node) {
	if n == nil {
		return
	}
	if n.Kind == yaml.DocumentNode {
		for _, child := range n.Content {
			setDoubleQuotes(child)
		}
		return
	}
	if n.Kind == yaml.MappingNode {
		for i := 0; i < len(n.Content); i += 2 {
			val := n.Content[i+1]
			if val.Kind == yaml.ScalarNode && val.Tag == "!!str" {
				val.Style = yaml.DoubleQuotedStyle
			} else {
				setDoubleQuotes(val)
			}
		}
		return
	}
	if n.Kind == yaml.SequenceNode {
		for _, item := range n.Content {
			if item.Kind == yaml.ScalarNode && item.Tag == "!!str" {
				item.Style = yaml.DoubleQuotedStyle
			} else {
				setDoubleQuotes(item)
			}
		}
		return
	}
}

// generateYAML marshals Config into YAML matching the target schema.
func generateYAML(cfg *Config) ([]byte, error) {
	var root yaml.Node
	if err := root.Encode(cfg); err != nil {
		return nil, fmt.Errorf("failed to encode YAML: %w", err)
	}
	setDoubleQuotes(&root)

	var buf bytes.Buffer
	enc := yaml.NewEncoder(&buf)
	enc.SetIndent(2)
	if err := enc.Encode(&root); err != nil {
		return nil, fmt.Errorf("failed to format YAML: %w", err)
	}
	_ = enc.Close()

	header := "---\n# Auto-generated by tools/proton-parser. DO NOT EDIT MANUALLY.\n"
	result := append([]byte(header), buf.Bytes()...)
	return result, nil
}

// processConfigs traverses the source directory, parses .conf files, and builds Config.
func processConfigs(inDir string) (*Config, error) {
	info, err := os.Stat(inDir)
	if err != nil {
		return nil, fmt.Errorf("source configs directory %q inaccessible: %w", inDir, err)
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("source path %q is not a directory", inDir)
	}

	entries, err := os.ReadDir(inDir)
	if err != nil {
		return nil, fmt.Errorf("failed to read directory %q: %w", inDir, err)
	}

	var accountDirs []string
	for _, entry := range entries {
		// Ignore hidden items and non-directories
		if strings.HasPrefix(entry.Name(), ".") || !entry.IsDir() {
			continue
		}
		accountDirs = append(accountDirs, entry.Name())
	}

	// Deterministic sorting of accounts
	sort.Strings(accountDirs)

	cfg := &Config{
		SingboxProtonAccounts: make([]Account, 0, len(accountDirs)),
	}

	for _, accName := range accountDirs {
		accPath := filepath.Join(inDir, accName)
		files, err := os.ReadDir(accPath)
		if err != nil {
			return nil, fmt.Errorf("failed to read account directory %q: %w", accPath, err)
		}

		var confFiles []string
		for _, f := range files {
			if f.IsDir() || strings.HasPrefix(f.Name(), ".") {
				continue
			}
			if strings.HasSuffix(strings.ToLower(f.Name()), ".conf") {
				confFiles = append(confFiles, filepath.Join(accPath, f.Name()))
			}
		}

		// Skip directories with zero .conf files with a logged warning
		if len(confFiles) == 0 {
			log.Printf("WARNING: account directory %q contains zero .conf files, skipping", accName)
			continue
		}

		// Deterministic sorting of config files
		sort.Strings(confFiles)

		nodes := make([]Node, 0, len(confFiles))
		for _, confFile := range confFiles {
			node, err := parseWireGuardConf(confFile, accName)
			if err != nil {
				return nil, err
			}
			nodes = append(nodes, node)
		}

		cfg.SingboxProtonAccounts = append(cfg.SingboxProtonAccounts, Account{
			Name:     accName,
			GroupTag: fmt.Sprintf("proton-group-%s", accName),
			Nodes:    nodes,
		})
	}

	return cfg, nil
}

// parseVaultPassFromAnsibleCfg attempts to extract vault_password_file from an ansible.cfg file.
func parseVaultPassFromAnsibleCfg(cfgPath string) string {
	f, err := os.Open(cfgPath)
	if err != nil {
		return ""
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if strings.HasPrefix(line, "#") || strings.HasPrefix(line, ";") {
			continue
		}
		if eqIdx := strings.Index(line, "="); eqIdx != -1 {
			key := strings.ToLower(strings.TrimSpace(line[:eqIdx]))
			if key == "vault_password_file" {
				val := stripQuotes(strings.TrimSpace(line[eqIdx+1:]))
				if val == "" {
					continue
				}
				if filepath.IsAbs(val) {
					if info, err := os.Stat(val); err == nil && !info.IsDir() {
						return val
					}
				} else {
					candidate := filepath.Clean(filepath.Join(filepath.Dir(cfgPath), val))
					if info, err := os.Stat(candidate); err == nil && !info.IsDir() {
						return candidate
					}
				}
			}
		}
	}
	return ""
}

// findDefaultVaultPassFile searches for an Ansible Vault password file across environment and parent directories.
func findDefaultVaultPassFile(outPath string) string {
	// 1. Environment variable
	if env := os.Getenv("ANSIBLE_VAULT_PASSWORD_FILE"); env != "" {
		if info, err := os.Stat(env); err == nil && !info.IsDir() {
			return env
		}
	}

	// 2. Search upwards from destination output directory
	if absOut, err := filepath.Abs(outPath); err == nil {
		dir := filepath.Dir(absOut)
		for {
			for _, name := range []string{".vault_pass", ".vault-password", ".vault_password"} {
				candidate := filepath.Join(dir, name)
				if info, err := os.Stat(candidate); err == nil && !info.IsDir() {
					return candidate
				}
			}
			cfgCandidate := filepath.Join(dir, "ansible.cfg")
			if passFromCfg := parseVaultPassFromAnsibleCfg(cfgCandidate); passFromCfg != "" {
				return passFromCfg
			}

			parent := filepath.Dir(dir)
			if parent == dir {
				break
			}
			dir = parent
		}
	}

	// 3. Search upwards from current working directory
	if cwd, err := os.Getwd(); err == nil {
		dir := cwd
		for {
			for _, name := range []string{".vault_pass", ".vault-password", ".vault_password"} {
				candidate := filepath.Join(dir, name)
				if info, err := os.Stat(candidate); err == nil && !info.IsDir() {
					return candidate
				}
				candidateAnsible := filepath.Join(dir, "ansible", name)
				if info, err := os.Stat(candidateAnsible); err == nil && !info.IsDir() {
					return candidateAnsible
				}
			}
			for _, cfgName := range []string{"ansible.cfg", filepath.Join("ansible", "ansible.cfg")} {
				cfgCandidate := filepath.Join(dir, cfgName)
				if passFromCfg := parseVaultPassFromAnsibleCfg(cfgCandidate); passFromCfg != "" {
					return passFromCfg
				}
			}

			parent := filepath.Dir(dir)
			if parent == dir {
				break
			}
			dir = parent
		}
	}

	return ""
}

// isEncryptedVault checks whether a file is already encrypted with Ansible Vault.
func isEncryptedVault(path string) (bool, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return false, err
	}
	return bytes.HasPrefix(data, []byte("$ANSIBLE_VAULT;")), nil
}

// encryptFile invokes ansible-vault encrypt on the specified file.
func encryptFile(outPath, vaultPassFile string) error {
	vaultBin, err := exec.LookPath("ansible-vault")
	if err != nil {
		return fmt.Errorf("ansible-vault executable not found in PATH: %w", err)
	}

	alreadyEncrypted, err := isEncryptedVault(outPath)
	if err == nil && alreadyEncrypted {
		log.Printf("File %s is already encrypted with ansible-vault, skipping", outPath)
		return nil
	}

	args := []string{"encrypt"}
	if vaultPassFile != "" {
		args = append(args, "--vault-password-file", vaultPassFile)
	} else {
		stat, err := os.Stdin.Stat()
		if err == nil && (stat.Mode()&os.ModeCharDevice) == 0 {
			return fmt.Errorf("cannot encrypt %s non-interactively: no vault password file specified or discovered (provide --vault-pass-file or set ANSIBLE_VAULT_PASSWORD_FILE)", outPath)
		}
	}
	args = append(args, outPath)

	cmd := exec.Command(vaultBin, args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Stdin = os.Stdin

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("ansible-vault encrypt failed: %w", err)
	}
	return nil
}

func main() {
	var inDir string
	var outPath string
	var encrypt bool
	var vaultPassFile string

	defaultIn := "./tools/proton-parser/configs"
	defaultOut := "./ansible/roles/singbox/vars/vault_proton.yml"

	// If invoked from inside tools/proton-parser directory (where go.mod exists)
	if _, err := os.Stat("go.mod"); err == nil {
		defaultIn = "./configs"
		defaultOut = "../../ansible/roles/singbox/vars/vault_proton.yml"
	}

	// Register flags with both short and long forms
	flag.StringVar(&inDir, "in", defaultIn, "Path to source configurations directory")
	flag.StringVar(&inDir, "i", defaultIn, "Path to source configurations directory (shorthand)")
	flag.StringVar(&outPath, "out", defaultOut, "Destination path for the generated YAML file")
	flag.StringVar(&outPath, "o", defaultOut, "Destination path for the generated YAML file (shorthand)")
	flag.BoolVar(&encrypt, "encrypt", false, "Encrypt output file with ansible-vault")
	flag.BoolVar(&encrypt, "e", false, "Encrypt output file with ansible-vault (shorthand)")
	flag.StringVar(&vaultPassFile, "vault-pass-file", "", "Path to Ansible Vault password file")
	flag.StringVar(&vaultPassFile, "vault-password-file", "", "Path to Ansible Vault password file (alias)")
	flag.StringVar(&vaultPassFile, "p", "", "Path to Ansible Vault password file (shorthand)")

	flag.Parse()

	// If default path wasn't found, fallback gracefully between relative structures
	if inDir == "./tools/proton-parser/configs" {
		if _, err := os.Stat(inDir); os.IsNotExist(err) {
			if _, err2 := os.Stat("./configs"); err2 == nil {
				inDir = "./configs"
			}
		}
	} else if inDir == "./configs" {
		if _, err := os.Stat(inDir); os.IsNotExist(err) {
			if _, err2 := os.Stat("./tools/proton-parser/configs"); err2 == nil {
				inDir = "./tools/proton-parser/configs"
			}
		}
	}

	if outPath == "./ansible/roles/singbox/vars/vault_proton.yml" {
		if _, err := os.Stat("go.mod"); err == nil {
			if _, err2 := os.Stat("../../ansible"); err2 == nil {
				outPath = "../../ansible/roles/singbox/vars/vault_proton.yml"
			}
		}
	} else if outPath == "../../ansible/roles/singbox/vars/vault_proton.yml" {
		if _, err := os.Stat("ansible"); err == nil {
			outPath = "./ansible/roles/singbox/vars/vault_proton.yml"
		}
	}

	cfg, err := processConfigs(inDir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	yamlData, err := generateYAML(cfg)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	outDir := filepath.Dir(outPath)
	if err := os.MkdirAll(outDir, 0755); err != nil {
		fmt.Fprintf(os.Stderr, "Error creating output directory %q: %v\n", outDir, err)
		os.Exit(1)
	}

	// Write generated YAML file with strict permissions (0600)
	if err := os.WriteFile(outPath, yamlData, 0600); err != nil {
		fmt.Fprintf(os.Stderr, "Error writing output file %q: %v\n", outPath, err)
		os.Exit(1)
	}

	totalAccounts := len(cfg.SingboxProtonAccounts)
	totalNodes := 0
	for _, acc := range cfg.SingboxProtonAccounts {
		totalNodes += len(acc.Nodes)
	}
	log.Printf("Successfully generated %s (%d accounts, %d nodes)", outPath, totalAccounts, totalNodes)

	if encrypt {
		if vaultPassFile == "" {
			if detected := findDefaultVaultPassFile(outPath); detected != "" {
				vaultPassFile = detected
				log.Printf("Automatically discovered vault password file: %s", vaultPassFile)
			}
		}
		if err := encryptFile(outPath, vaultPassFile); err != nil {
			_ = os.Remove(outPath)
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}
		log.Printf("Successfully encrypted %s with ansible-vault", outPath)
	}
}
