package main

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/spf13/cobra"
)

func init() {
	cmd := &cobra.Command{
		Use:   "init",
		Short: "Interactive setup wizard to configure frm",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runInit(cmd.InOrStdin(), cmd.OutOrStdout())
		},
	}
	rootCmd.AddCommand(cmd)
}

// prompt reads a line from the scanner, printing the given message first.
// Returns the trimmed input. If the scanner hits EOF, returns empty string and io.EOF.
func prompt(scanner *bufio.Scanner, w io.Writer, msg string) (string, error) {
	fmt.Fprint(w, msg)
	if !scanner.Scan() {
		if err := scanner.Err(); err != nil {
			return "", err
		}
		return "", io.EOF
	}
	return strings.TrimSpace(scanner.Text()), nil
}

func runInit(r io.Reader, w io.Writer) error {
	scanner := bufio.NewScanner(r)

	path := configPath()
	dir := configDir()

	var existing *Config
	if data, err := os.ReadFile(path); err == nil {
		var cfg Config
		if err := json.Unmarshal(data, &cfg); err == nil {
			existing = &cfg
		}
	}

	var services []ServiceConfig

	if existing != nil {
		fmt.Fprintf(w, "Config file already exists at %s\n", path)
		answer, err := prompt(scanner, w, "Do you want to (o)verwrite or (a)dd a service? [o/a]: ")
		if err != nil {
			return err
		}
		switch strings.ToLower(answer) {
		case "a", "add":
			services = existing.Services
		case "o", "overwrite":
			// start fresh
		default:
			return fmt.Errorf("invalid choice %q, expected 'o' or 'a'", answer)
		}
	}

	// Prompt for service type
	svcType, err := prompt(scanner, w, "Service type: (c)arddav or (j)map? [c]: ")
	if err != nil {
		return err
	}
	svcType = strings.ToLower(svcType)
	if svcType == "" {
		svcType = "c"
	}

	switch svcType {
	case "c", "carddav":
		svc, err := promptCardDAV(scanner, w)
		if err != nil {
			return err
		}
		services = append(services, svc)
	case "j", "jmap":
		svc, err := promptJMAP(scanner, w)
		if err != nil {
			return err
		}
		services = append(services, svc)
	default:
		return fmt.Errorf("unknown service type %q", svcType)
	}

	cfg := Config{Services: services}

	// Write config
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("creating config directory: %w", err)
	}
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return fmt.Errorf("marshaling config: %w", err)
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		return fmt.Errorf("writing config: %w", err)
	}

	fmt.Fprintf(w, "\nConfig written to %s\n", path)

	// Ask about adding JMAP if we just added CardDAV
	if services[len(services)-1].Type == "carddav" {
		addJMAP, err := prompt(scanner, w, "Add JMAP service for email context? [y/N]: ")
		if err == nil && strings.ToLower(addJMAP) == "y" {
			svc, err := promptJMAP(scanner, w)
			if err != nil {
				return err
			}
			cfg.Services = append(cfg.Services, svc)
			data, err = json.MarshalIndent(cfg, "", "  ")
			if err != nil {
				return fmt.Errorf("marshaling config: %w", err)
			}
			if err := os.WriteFile(path, data, 0o600); err != nil {
				return fmt.Errorf("writing config: %w", err)
			}
			fmt.Fprintf(w, "Config updated with JMAP service.\n")
		}
	}

	fmt.Fprintf(w, "\nRun 'frm triage' to start categorizing your contacts\n")
	return nil
}

func promptCardDAV(scanner *bufio.Scanner, w io.Writer) (ServiceConfig, error) {
	fmt.Fprintln(w, "\nCardDAV Setup")
	fmt.Fprintln(w, "Choose a provider preset:")
	fmt.Fprintln(w, "  1) iCloud (needs app-specific password)")
	fmt.Fprintln(w, "  2) Fastmail")
	fmt.Fprintln(w, "  3) Custom URL")

	choice, err := prompt(scanner, w, "Provider [1/2/3]: ")
	if err != nil {
		return ServiceConfig{}, err
	}

	var endpoint string
	switch choice {
	case "1", "icloud":
		endpoint = "https://contacts.icloud.com"
		fmt.Fprintln(w, "Using iCloud endpoint. You will need an app-specific password.")
		fmt.Fprintln(w, "Generate one at https://appleid.apple.com/account/manage")
	case "2", "fastmail":
		endpoint = "https://carddav.fastmail.com"
		fmt.Fprintln(w, "Using Fastmail endpoint.")
	case "3", "custom":
		endpoint, err = prompt(scanner, w, "CardDAV endpoint URL: ")
		if err != nil {
			return ServiceConfig{}, err
		}
	default:
		return ServiceConfig{}, fmt.Errorf("invalid provider choice %q", choice)
	}

	if endpoint == "" {
		return ServiceConfig{}, fmt.Errorf("endpoint URL is required")
	}

	username, err := prompt(scanner, w, "Username: ")
	if err != nil {
		return ServiceConfig{}, err
	}
	if username == "" {
		return ServiceConfig{}, fmt.Errorf("username is required")
	}

	fmt.Fprintln(w, "Note: password will be visible as you type (no terminal raw mode).")
	password, err := prompt(scanner, w, "Password: ")
	if err != nil {
		return ServiceConfig{}, err
	}
	if password == "" {
		return ServiceConfig{}, fmt.Errorf("password is required")
	}

	svc := ServiceConfig{
		Type:     "carddav",
		Endpoint: endpoint,
		Username: username,
		Password: password,
	}

	// Validate connection
	fmt.Fprintln(w, "Validating connection...")
	client, err := newCardDAVClient(svc)
	if err != nil {
		fmt.Fprintf(w, "Warning: could not connect to CardDAV server: %v\n", err)
		save, promptErr := prompt(scanner, w, "Save config anyway? [y/N]: ")
		if promptErr != nil {
			return ServiceConfig{}, promptErr
		}
		if strings.ToLower(save) != "y" {
			return ServiceConfig{}, fmt.Errorf("aborted")
		}
		return svc, nil
	}

	ctx := context.Background()
	_, err = findAddressBook(ctx, client)
	if err != nil {
		fmt.Fprintf(w, "Warning: connected but could not find address books: %v\n", err)
		save, promptErr := prompt(scanner, w, "Save config anyway? [y/N]: ")
		if promptErr != nil {
			return ServiceConfig{}, promptErr
		}
		if strings.ToLower(save) != "y" {
			return ServiceConfig{}, fmt.Errorf("aborted")
		}
		return svc, nil
	}

	fmt.Fprintln(w, "Connection successful! Address book found.")
	return svc, nil
}

func promptJMAP(scanner *bufio.Scanner, w io.Writer) (ServiceConfig, error) {
	fmt.Fprintln(w, "\nJMAP Setup")

	endpoint, err := prompt(scanner, w, "JMAP session endpoint URL: ")
	if err != nil {
		return ServiceConfig{}, err
	}
	if endpoint == "" {
		return ServiceConfig{}, fmt.Errorf("session endpoint is required")
	}

	fmt.Fprintln(w, "Note: token will be visible as you type (no terminal raw mode).")
	token, err := prompt(scanner, w, "API token: ")
	if err != nil {
		return ServiceConfig{}, err
	}
	if token == "" {
		return ServiceConfig{}, fmt.Errorf("token is required")
	}

	return ServiceConfig{
		Type:            "jmap",
		SessionEndpoint: endpoint,
		Token:           token,
	}, nil
}
