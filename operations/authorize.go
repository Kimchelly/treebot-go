package operations

import (
	"github.com/urfave/cli/v2"
)

func Authorize() *cli.Command {
	return &cli.Command{
		Name:  "authorize",
		Usage: "manually authorize a Dependabot PR",
		// Flags: []cli.Flag{
		//     cli.StringFlag{
		//         Name:
		//     },
		// cli.BoolFlag{ Name: "all",
		//     Usage: "authorize all available Dependabot PRs from notifications",
		// },
		// },
		// Action: func(c *cli.Context) error {
		//     return nil,
		// },
	}
}
