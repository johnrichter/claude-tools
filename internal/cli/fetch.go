package cli

import (
	"time"

	"github.com/spf13/cobra"

	"github.com/johnrichter/claude-shared-tooling/go/webfetch"
)

func newFetchCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "fetch",
		Short: "Fetch a site's sitemap, article metadata, or feed entries",
	}
	cmd.AddCommand(newFetchSitemapCmd())
	cmd.AddCommand(newFetchArticleCmd())
	cmd.AddCommand(newFetchFeedCmd())
	return cmd
}

func newFetcher(cmd *cobra.Command) (*webfetch.Fetcher, error) {
	cfg, err := loadConfig(cmd.Flags())
	if err != nil {
		return nil, err
	}
	return webfetch.NewFetcher(cfg.Timeout), nil
}

func newFetchSitemapCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "sitemap <url>",
		Short:   "List sitemap entries, optionally windowed by path prefix and modification date",
		Args:    cobra.ExactArgs(1),
		Example: "  claude-tools fetch sitemap https://example.com/sitemap.xml --path-prefix /blog/ --since 2026-01-01",
		RunE: func(cmd *cobra.Command, args []string) error {
			f, err := newFetcher(cmd)
			if err != nil {
				return finishErr(cmd, "internal.config.load_failed", "load configuration", err)
			}
			pathPrefix, _ := cmd.Flags().GetString("path-prefix")
			since, _ := cmd.Flags().GetString("since")
			until, _ := cmd.Flags().GetString("until")

			window := webfetch.SitemapWindow{PathPrefix: pathPrefix}
			if since != "" {
				t, parseErr := time.Parse("2006-01-02", since)
				if parseErr != nil {
					return finishUsage(cmd, "usage.fetch.invalid_since", "--since must be an RFC 3339 date (YYYY-MM-DD)")
				}
				window.Since = t
			}
			if until != "" {
				t, parseErr := time.Parse("2006-01-02", until)
				if parseErr != nil {
					return finishUsage(cmd, "usage.fetch.invalid_until", "--until must be an RFC 3339 date (YYYY-MM-DD)")
				}
				window.Until = t
			}

			res, fetchErr := f.FetchSitemap(cmd.Context(), args[0], window)
			if fetchErr != nil {
				return finishErr(cmd, "internal.webfetch.sitemap_failed", "fetch sitemap", fetchErr)
			}

			entries := make([]map[string]any, 0, len(res.Entries))
			for _, e := range res.Entries {
				entries = append(entries, map[string]any{"url": e.URL, "last_mod": e.LastMod})
			}
			result, buildErr := clikitSuccess(cmd, map[string]any{"entries": entries, "skipped": res.Skipped})
			if buildErr != nil {
				return finishErr(cmd, "internal.result.build_failed", "build result", buildErr)
			}
			return finish(cmd, result)
		},
	}
	cmd.Flags().String("path-prefix", "", "only include sitemap URLs whose path starts with this prefix")
	cmd.Flags().String("since", "", "only include entries last-modified on or after this date (YYYY-MM-DD)")
	cmd.Flags().String("until", "", "only include entries last-modified on or before this date (YYYY-MM-DD)")
	return cmd
}

func newFetchArticleCmd() *cobra.Command {
	return &cobra.Command{
		Use:     "article <url>",
		Short:   "Extract an article's title, description, author and publish date",
		Args:    cobra.ExactArgs(1),
		Example: "  claude-tools fetch article https://example.com/post",
		RunE: func(cmd *cobra.Command, args []string) error {
			f, err := newFetcher(cmd)
			if err != nil {
				return finishErr(cmd, "internal.config.load_failed", "load configuration", err)
			}
			meta, fetchErr := f.FetchArticleMeta(cmd.Context(), args[0])
			if fetchErr != nil {
				return finishErr(cmd, "internal.webfetch.article_failed", "fetch article metadata", fetchErr)
			}
			data := map[string]any{
				"url":          meta.URL,
				"title":        meta.Title,
				"description":  meta.Description,
				"author":       meta.Author,
				"published_at": meta.PublishedAt,
				"missing":      meta.Missing,
			}
			result, buildErr := clikitSuccess(cmd, data)
			if buildErr != nil {
				return finishErr(cmd, "internal.result.build_failed", "build result", buildErr)
			}
			return finish(cmd, result)
		},
	}
}

func newFetchFeedCmd() *cobra.Command {
	return &cobra.Command{
		Use:     "feed <url>",
		Short:   "List an RSS/Atom feed's entries",
		Args:    cobra.ExactArgs(1),
		Example: "  claude-tools fetch feed https://example.com/feed.xml",
		RunE: func(cmd *cobra.Command, args []string) error {
			f, err := newFetcher(cmd)
			if err != nil {
				return finishErr(cmd, "internal.config.load_failed", "load configuration", err)
			}
			entries, fetchErr := f.FetchFeed(cmd.Context(), args[0])
			if fetchErr != nil {
				return finishErr(cmd, "internal.webfetch.feed_failed", "fetch feed", fetchErr)
			}
			out := make([]map[string]any, 0, len(entries))
			for _, e := range entries {
				out = append(out, map[string]any{
					"title":     e.Title,
					"url":       e.URL,
					"published": e.Published,
					"kind":      e.Kind,
					"missing":   e.Missing,
				})
			}
			result, buildErr := clikitSuccess(cmd, map[string]any{"entries": out})
			if buildErr != nil {
				return finishErr(cmd, "internal.result.build_failed", "build result", buildErr)
			}
			return finish(cmd, result)
		},
	}
}
