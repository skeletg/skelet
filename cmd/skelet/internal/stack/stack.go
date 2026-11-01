// Package stack holds the verbs that act on the compose stack as a whole, and serve, which runs a frontend beside it
package stack

import (
	"github.com/skeletg/skelet/cmd/skelet/internal/compose"
)

const (
	DownAbout = `Kills and removes the containers - your data (volumes) survives. --remove-orphans also sweeps
the apps you've deselected in skelet setup. Arguments go to docker compose down.`
	StopAbout    = `Stops everything without removing it - skelet up brings it all back. Arguments go to docker compose stop.`
	RestartAbout = `Restarts containers in place. A full recreate is skelet down, then skelet up. Arguments go to docker compose restart.`
	PsAbout      = `What's running, and on which ports. Arguments go to docker compose ps.`
	LogsAbout    = `Follows the logs - skelet logs app, skelet logs --tail 50 garage. Arguments go to docker compose logs -f.`
)

// The compose passthroughs become docker compose itself, with the profiles that follow your app and tool selection
// - so they cover what setup picked and nothing it didn't

func Down(root string, args []string) int    { return passthrough(root, "down", "--remove-orphans", args) }
func Stop(root string, args []string) int    { return passthrough(root, "stop", "", args) }
func Restart(root string, args []string) int { return passthrough(root, "restart", "", args) }
func Ps(root string, args []string) int      { return passthrough(root, "ps", "", args) }
func Logs(root string, args []string) int    { return passthrough(root, "logs", "-f", args) }

func passthrough(root, sub, flag string, args []string) int {
	argv := []string{sub}
	if flag != "" {
		argv = append(argv, flag)
	}
	lib := compose.Load(root)
	return compose.Call(root, lib.Env(), append(argv, args...)...).Replace()
}
