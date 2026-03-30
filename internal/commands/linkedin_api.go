package commands

import (
	"bufio"
	"fmt"
	"os"
	"strings"

	"github.com/pkg/browser"
	"github.com/spf13/cobra"

	"github.com/noko/computecommander/internal/linkedin"
)

// linkedinSetupCmd configures LinkedIn API OAuth2 credentials and authorizes the app.
func linkedinSetupCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "setup",
		Short: "Configure LinkedIn API credentials",
		Long: `Set up LinkedIn OAuth2 credentials for automated publishing.

Prerequisites:
  1. Create a LinkedIn Developer App at https://www.linkedin.com/developers/apps
  2. Add the "Share on LinkedIn" and "Sign In with LinkedIn using OpenID Connect" products
  3. Set the redirect URI to http://localhost:9876/callback (or your preferred URI)
  4. Note your Client ID and Client Secret

This command will:
  - Save your credentials to ~/.config/computecommander/linkedin/oauth.json (mode 0600)
  - Open a browser for authorization
  - Exchange the authorization code for access/refresh tokens`,
		RunE: func(cmd *cobra.Command, args []string) error {
			reader := bufio.NewReader(os.Stdin)

			fmt.Println("LinkedIn API Setup")
			fmt.Println("==================")
			fmt.Println()
			fmt.Println("Create a LinkedIn app at: https://www.linkedin.com/developers/apps")
			fmt.Println("Required products: 'Share on LinkedIn' + 'Sign In with LinkedIn using OpenID Connect'")
			fmt.Println()

			fmt.Print("Client ID: ")
			clientID, _ := reader.ReadString('\n')
			clientID = strings.TrimSpace(clientID)

			fmt.Print("Client Secret: ")
			clientSecret, _ := reader.ReadString('\n')
			clientSecret = strings.TrimSpace(clientSecret)

			redirectURI := "http://localhost:9876/callback"
			fmt.Printf("Redirect URI [%s]: ", redirectURI)
			customURI, _ := reader.ReadString('\n')
			customURI = strings.TrimSpace(customURI)
			if customURI != "" {
				redirectURI = customURI
			}

			if clientID == "" || clientSecret == "" {
				return fmt.Errorf("client ID and client secret are required")
			}

			// Save credentials.
			if err := linkedin.SetupCredentials(clientID, clientSecret, redirectURI); err != nil {
				return fmt.Errorf("save credentials: %w", err)
			}
			fmt.Println("Credentials saved to ~/.config/computecommander/linkedin/oauth.json")

			// Create API client and start auth flow.
			client, err := linkedin.NewAPIClient()
			if err != nil {
				return fmt.Errorf("create API client: %w", err)
			}

			authURL := client.AuthURL()
			fmt.Println()
			fmt.Println("Opening browser for authorization...")
			fmt.Println("If the browser does not open, visit this URL:")
			fmt.Println(authURL)
			fmt.Println()

			_ = browser.OpenURL(authURL)

			fmt.Print("Paste the authorization code from the redirect URL: ")
			code, _ := reader.ReadString('\n')
			code = strings.TrimSpace(code)

			if code == "" {
				return fmt.Errorf("authorization code is required")
			}

			if err := client.ExchangeCode(code); err != nil {
				return fmt.Errorf("exchange code: %w", err)
			}

			fmt.Println()
			fmt.Println("LinkedIn API setup complete!")
			fmt.Printf("Token status: %s\n", client.TokenStatus())
			fmt.Println("You can now publish approved posts with: cmdr linkedin publish <id>")

			return nil
		},
	}

	return cmd
}

// linkedinPublishCmd publishes an approved post to LinkedIn.
func linkedinPublishCmd(app *App) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "publish <id>",
		Short: "Publish an approved post to LinkedIn",
		Long: `Publish a post that has been approved via 'cmdr linkedin approve'.

The post must have status 'approved'. After successful publishing,
the post status is updated to 'posted' with a timestamp.

Requires LinkedIn API to be configured via 'cmdr linkedin setup'.`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			id, err := parsePostID(args[0])
			if err != nil {
				return err
			}

			gen := newGenerator(app)
			store := gen.PostStore()

			// Fetch the post.
			post, err := store.Get(id)
			if err != nil {
				return fmt.Errorf("get post %d: %w", id, err)
			}

			if post.Status != linkedin.StatusApproved {
				return fmt.Errorf("post %d has status %q; must be 'approved' to publish", id, post.Status)
			}

			// Check if LinkedIn API is configured.
			if !linkedin.IsConfigured() {
				return fmt.Errorf("LinkedIn API not configured. Run: cmdr linkedin setup")
			}

			client, err := linkedin.NewAPIClient()
			if err != nil {
				return err
			}

			dryRun, _ := cmd.Flags().GetBool("dry-run")
			if dryRun {
				fmt.Println("DRY RUN -- would publish to LinkedIn:")
				fmt.Println()
				fmt.Println(post.Content)
				fmt.Println()
				fmt.Printf("Character count: %d\n", len(post.Content))
				return nil
			}

			fmt.Printf("Publishing post #%d to LinkedIn...\n", id)
			postURN, err := client.Publish(post.Content)
			if err != nil {
				return fmt.Errorf("publish to linkedin: %w", err)
			}

			// Update status to posted.
			if err := store.MarkPosted(id); err != nil {
				fmt.Printf("Warning: failed to update post status: %v\n", err)
			}

			fmt.Printf("Published successfully!\n")
			fmt.Printf("  Post ID:  %d\n", id)
			fmt.Printf("  LinkedIn: %s\n", linkedin.PostURL(postURN))
			fmt.Println()
			fmt.Printf("Record engagement later: cmdr linkedin feedback %d <1-5> --notes 'engagement notes'\n", id)

			return nil
		},
	}

	cmd.Flags().Bool("dry-run", false, "Preview without publishing")
	return cmd
}

// linkedinStatusCmd shows the current LinkedIn API configuration and token status.
func linkedinStatusCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "status",
		Short: "Show LinkedIn API status",
		RunE: func(cmd *cobra.Command, args []string) error {
			fmt.Println("LinkedIn API Status")
			fmt.Println("===================")
			fmt.Println()

			if !linkedin.IsConfigured() {
				fmt.Println("Status: NOT CONFIGURED")
				fmt.Println("Run: cmdr linkedin setup")
				return nil
			}

			fmt.Println("Credentials: configured")

			client, err := linkedin.NewAPIClient()
			if err != nil {
				fmt.Printf("Config error: %v\n", err)
				return nil
			}

			fmt.Printf("Token: %s\n", client.TokenStatus())
			fmt.Printf("Config: %s/oauth.json\n", linkedin.DefaultConfigDir())

			return nil
		},
	}
}

// parsePostID converts a string argument to a post ID.
func parsePostID(s string) (int64, error) {
	var id int64
	_, err := fmt.Sscanf(s, "%d", &id)
	if err != nil {
		return 0, fmt.Errorf("invalid post ID %q: must be a number", s)
	}
	return id, nil
}
