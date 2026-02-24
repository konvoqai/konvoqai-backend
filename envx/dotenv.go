package envx

import (
	"bufio"
	"errors"
	"os"
	"strings"
)

// LoadDotEnvIfPresent loads KEY=VALUE pairs from filePath into process env.
// Existing environment variables are preserved.
func LoadDotEnvIfPresent(filePath string) error {
	return loadDotEnv(filePath, false)
}

// LoadDotEnvOverrideIfPresent loads .env and overwrites already-set env vars.
func LoadDotEnvOverrideIfPresent(filePath string) error {
	return loadDotEnv(filePath, true)
}

func loadDotEnv(filePath string, overwrite bool) error {
	f, err := os.Open(filePath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return err
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if strings.HasPrefix(line, "export ") {
			line = strings.TrimSpace(strings.TrimPrefix(line, "export "))
		}
		i := strings.Index(line, "=")
		if i <= 0 {
			continue
		}

		key := strings.TrimSpace(line[:i])
		val := strings.TrimSpace(line[i+1:])
		if key == "" {
			continue
		}
		if len(val) >= 2 {
			if (val[0] == '"' && val[len(val)-1] == '"') || (val[0] == '\'' && val[len(val)-1] == '\'') {
				val = val[1 : len(val)-1]
			}
		}
		if !overwrite {
			if _, exists := os.LookupEnv(key); exists {
				continue
			}
		}
		if overwrite {
			if err := os.Setenv(key, val); err != nil {
				return err
			}
			continue
		}
		_ = os.Setenv(key, val)
	}

	return scanner.Err()
}
