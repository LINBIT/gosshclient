package sshclient

import (
	"fmt"
	"strings"
)

// AddEnv takes a script and prepends it with export lines from the env variable slice.
// Entries in env should have a VAR=val syntax
// Note that this currently does not do any checks!
// There is a ssh.session.Setenv(), but usually ssh, for good reasons, does only allow to set a
// limited set of variables/no user variables at all.
func AddEnv(script string, env []string) (string, error) {
	var envBlob string
	for _, kv := range env {
		kvs := strings.SplitN(kv, "=", 2)
		envBlob += fmt.Sprintln(kv, "; export ", kvs[0])
	}

	return fmt.Sprint(envBlob, script), nil
}
