package main

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

var forceInit bool

var initCmd = &cobra.Command{
	Use:   "init",
	Short: "Scaffold a starter rute.yaml in the current directory",
	RunE:  runInit,
}

func init() {
	initCmd.Flags().BoolVar(&forceInit, "force", false, "overwrite existing rute.yaml")
	rootCmd.AddCommand(initCmd)
}

const starterYAML = `title: My API
version: 1.0.0
# baseUrl: https://api.example.com

endpoints:
  # - path: /users/:id
  #   method: GET
  #   description: Get a user by ID
  #   params:
  #     schema: ./schemas/user.ts#UserParamsSchema
  #   response:
  #     200:
  #       schema: ./schemas/user.ts#UserResponseSchema

  - path: /health
    method: GET
    description: Health check endpoint
`

func runInit(cmd *cobra.Command, args []string) error {
	const filename = "rute.yaml"

	if !forceInit {
		if _, err := os.Stat(filename); err == nil {
			return fmt.Errorf("%s already exists (use --force to overwrite)", filename)
		}
	}

	if err := os.WriteFile(filename, []byte(starterYAML), 0644); err != nil {
		return fmt.Errorf("failed to write %s: %w", filename, err)
	}

	fmt.Printf("Created %s\n", filename)
	return nil
}
