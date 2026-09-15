// ALR - Any Linux Repository
// Copyright (C) 2025 The ALR Authors
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// This program is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
// GNU General Public License for more details.
//
// You should have received a copy of the GNU General Public License
// along with this program.  If not, see <http://www.gnu.org/licenses/>.

package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/leonelquinteros/gotext"
	"github.com/spf13/cobra"
	"golang.org/x/exp/slices"

	"git.alr-pkg.ru/ALR/ALR/internal/build"
	appbuilder "git.alr-pkg.ru/ALR/ALR/internal/cliutils/app_builder"
	"git.alr-pkg.ru/ALR/ALR/internal/utils"
	"git.alr-pkg.ru/ALR/ALR/pkg/types"
)

func newRepoCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "repo",
		Short: gotext.Get("Manage repos"),
	}

	cmd.AddCommand(
		newRepoRemoveCmd(),
		newRepoAddCmd(),
		newRepoSetRefCmd(),
		newRepoSetUrlCmd(),
		newRepoMirrorCmd(),
	)

	addFlagCompletion(cmd)
	return cmd
}

func newRepoRemoveCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:               "remove",
		Short:             gotext.Get("Remove an existing repository"),
		Aliases:           []string{"rm"},
		Args:              cobra.ExactArgs(1),
		ValidArgsFunction: repoNameCompletion,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runRepoRemove(cmd, args[0])
		},
	}

	addFlagCompletion(cmd)
	return cmd
}

func runRepoRemove(cmd *cobra.Command, name string) error {
	if err := utils.CheckUserPrivileges(); err != nil {
		return err
	}

	ctx := cmd.Context()

	deps, err := appbuilder.
		New(ctx).
		WithConfig().
		Build()
	if err != nil {
		return err
	}
	defer deps.Defer()

	cfg := deps.Cfg

	found := false
	index := 0
	reposSlice := cfg.Repos()
	for i, repo := range reposSlice {
		if repo.Name == name {
			index = i
			found = true
		}
	}
	if !found {
		return fmt.Errorf("%s", gotext.Get("Repo \"%s\" does not exist", name))
	}

	cfg.SetRepos(slices.Delete(reposSlice, index, index+1))

	err = os.RemoveAll(filepath.Join(cfg.GetPaths().RepoDir, name))
	if err != nil {
		return fmt.Errorf("%s: %w", gotext.Get("Error removing repo directory"), err)
	}
	err = cfg.System.Save()
	if err != nil {
		return fmt.Errorf("%s: %w", gotext.Get("Error saving config"), err)
	}

	deps, err = appbuilder.
		New(ctx).
		UseConfig(cfg).
		WithDB().
		Build()
	if err != nil {
		return err
	}
	defer deps.Defer()

	err = deps.DB.DeletePkgs(ctx, "repository = ?", name)
	if err != nil {
		return fmt.Errorf("%s: %w", gotext.Get("Error removing packages from database"), err)
	}

	return nil
}

func newRepoAddCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "add",
		Short: gotext.Get("Add a new repository"),
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runRepoAdd(cmd, args[0], args[1])
		},
	}

	addFlagCompletion(cmd)
	return cmd
}

func runRepoAdd(cmd *cobra.Command, name, repoURL string) error {
	if err := utils.CheckUserPrivileges(); err != nil {
		return err
	}

	ctx := cmd.Context()

	deps, err := appbuilder.
		New(ctx).
		WithConfig().
		Build()
	if err != nil {
		return err
	}
	defer deps.Defer()

	cfg := deps.Cfg

	reposSlice := cfg.Repos()
	for _, repo := range reposSlice {
		if repo.URL == repoURL || repo.Name == name {
			return fmt.Errorf("%s", gotext.Get("Repo \"%s\" already exists", repo.Name))
		}
	}

	newRepo := types.Repo{
		Name: name,
		URL:  repoURL,
	}

	r, close, err := build.GetSafeReposExecutor()
	if err != nil {
		return err
	}
	defer close()

	newRepo, err = r.PullOneAndUpdateFromConfig(ctx, &newRepo)
	if err != nil {
		return err
	}

	reposSlice = append(reposSlice, newRepo)
	cfg.SetRepos(reposSlice)

	err = cfg.System.Save()
	if err != nil {
		return fmt.Errorf("%s: %w", gotext.Get("Error saving config"), err)
	}

	return nil
}

func newRepoSetRefCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "set-ref",
		Short: gotext.Get("Set the reference of the repository"),
		Args:  cobra.ExactArgs(2),
		ValidArgsFunction: func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
			if len(args) > 0 {
				return nil, cobra.ShellCompDirectiveNoFileComp
			}
			return repoNameCompletion(cmd, args, toComplete)
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := utils.CheckUserPrivileges(); err != nil {
				return err
			}

			name := args[0]
			ref := args[1]

			deps, err := appbuilder.
				New(cmd.Context()).
				WithConfig().
				WithDB().
				WithReposNoPull().
				Build()
			if err != nil {
				return err
			}
			defer deps.Defer()

			repos := deps.Cfg.Repos()
			newRepos := []types.Repo{}
			for _, repo := range repos {
				if repo.Name == name {
					repo.Ref = ref
				}
				newRepos = append(newRepos, repo)
			}
			deps.Cfg.SetRepos(newRepos)
			err = deps.Cfg.System.Save()
			if err != nil {
				return fmt.Errorf("%s: %w", gotext.Get("Error saving config"), err)
			}

			err = deps.Repos.Pull(cmd.Context(), newRepos)
			if err != nil {
				return fmt.Errorf("%s: %w", gotext.Get("Error pulling repositories"), err)
			}

			return nil
		},
	}

	addFlagCompletion(cmd)
	return cmd
}

func newRepoSetUrlCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "set-url",
		Short: gotext.Get("Set the main url of the repository"),
		Args:  cobra.ExactArgs(2),
		ValidArgsFunction: func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
			if len(args) > 0 {
				return nil, cobra.ShellCompDirectiveNoFileComp
			}
			return repoNameCompletion(cmd, args, toComplete)
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := utils.CheckUserPrivileges(); err != nil {
				return err
			}

			name := args[0]
			repoUrl := args[1]

			deps, err := appbuilder.
				New(cmd.Context()).
				WithConfig().
				WithDB().
				WithReposNoPull().
				Build()
			if err != nil {
				return err
			}
			defer deps.Defer()

			repos := deps.Cfg.Repos()
			newRepos := []types.Repo{}
			for _, repo := range repos {
				if repo.Name == name {
					repo.URL = repoUrl
				}
				newRepos = append(newRepos, repo)
			}
			deps.Cfg.SetRepos(newRepos)
			err = deps.Cfg.System.Save()
			if err != nil {
				return fmt.Errorf("%s: %w", gotext.Get("Error saving config"), err)
			}

			err = deps.Repos.Pull(cmd.Context(), newRepos)
			if err != nil {
				return fmt.Errorf("%s: %w", gotext.Get("Error pulling repositories"), err)
			}

			return nil
		},
	}

	addFlagCompletion(cmd)
	return cmd
}

func newRepoMirrorCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "mirror",
		Short: gotext.Get("Manage mirrors of repos"),
	}

	cmd.AddCommand(
		newRepoMirrorAddCmd(),
		newRepoMirrorRemoveCmd(),
		newRepoMirrorClearCmd(),
	)

	addFlagCompletion(cmd)
	return cmd
}

func newRepoMirrorAddCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "add",
		Short: gotext.Get("Add a mirror URL to repository"),
		Args:  cobra.ExactArgs(2),
		ValidArgsFunction: func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
			if len(args) > 0 {
				return nil, cobra.ShellCompDirectiveNoFileComp
			}
			return repoNameCompletion(cmd, args, toComplete)
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := utils.CheckUserPrivileges(); err != nil {
				return err
			}

			name := args[0]
			url := args[1]

			deps, err := appbuilder.
				New(cmd.Context()).
				WithConfig().
				WithDB().
				WithReposNoPull().
				Build()
			if err != nil {
				return err
			}
			defer deps.Defer()

			repos := deps.Cfg.Repos()
			for i, repo := range repos {
				if repo.Name == name {
					repos[i].Mirrors = append(repos[i].Mirrors, url)
					break
				}
			}
			deps.Cfg.SetRepos(repos)
			err = deps.Cfg.System.Save()
			if err != nil {
				return fmt.Errorf("%s: %w", gotext.Get("Error saving config"), err)
			}

			return nil
		},
	}

	addFlagCompletion(cmd)
	return cmd
}

func newRepoMirrorRemoveCmd() *cobra.Command {
	var (
		ignoreMissing bool
		partial       bool
	)

	cmd := &cobra.Command{
		Use:     "remove",
		Short:   gotext.Get("Remove mirror from the repository"),
		Aliases: []string{"rm"},
		Args:    cobra.ExactArgs(2),
		ValidArgsFunction: func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
			if len(args) > 0 {
				return nil, cobra.ShellCompDirectiveNoFileComp
			}
			return repoNameCompletion(cmd, args, toComplete)
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := utils.CheckUserPrivileges(); err != nil {
				return err
			}

			name := args[0]
			urlToRemove := args[1]

			deps, err := appbuilder.
				New(cmd.Context()).
				WithConfig().
				WithDB().
				WithReposNoPull().
				Build()
			if err != nil {
				return err
			}
			defer deps.Defer()

			reposSlice := deps.Cfg.Repos()
			repoIndex := -1
			urlIndicesToRemove := []int{}

			// Находим репозиторий
			for i, repo := range reposSlice {
				if repo.Name == name {
					repoIndex = i
					break
				}
			}

			if repoIndex == -1 {
				if ignoreMissing {
					return nil // Тихо завершаем, если репозиторий не найден
				}
				return fmt.Errorf("%s", gotext.Get("Repo \"%s\" does not exist", name))
			}

			// Ищем зеркала для удаления
			repo := reposSlice[repoIndex]
			for j, mirror := range repo.Mirrors {
				var match bool
				if partial {
					// Частичное совпадение - проверяем, содержит ли зеркало указанную строку
					match = strings.Contains(mirror, urlToRemove)
				} else {
					// Точное совпадение
					match = mirror == urlToRemove
				}

				if match {
					urlIndicesToRemove = append(urlIndicesToRemove, j)
				}
			}

			if len(urlIndicesToRemove) == 0 {
				if ignoreMissing {
					return nil
				}
				if partial {
					return fmt.Errorf("%s", gotext.Get("No mirrors containing \"%s\" found in repo \"%s\"", urlToRemove, name))
				}
				return fmt.Errorf("%s", gotext.Get("URL \"%s\" does not exist in repo \"%s\"", urlToRemove, name))
			}

			for i := len(urlIndicesToRemove) - 1; i >= 0; i-- {
				urlIndex := urlIndicesToRemove[i]
				reposSlice[repoIndex].Mirrors = slices.Delete(reposSlice[repoIndex].Mirrors, urlIndex, urlIndex+1)
			}

			deps.Cfg.SetRepos(reposSlice)
			err = deps.Cfg.System.Save()
			if err != nil {
				return fmt.Errorf("%s: %w", gotext.Get("Error saving config"), err)
			}

			if len(urlIndicesToRemove) > 1 {
				fmt.Println(gotext.Get("Removed %d mirrors from repo \"%s\"\n", len(urlIndicesToRemove), name))
			}

			return nil
		},
	}

	cmd.Flags().BoolVar(&ignoreMissing, "ignore-missing", false, gotext.Get("Ignore if mirror does not exist"))
	cmd.Flags().BoolVarP(&partial, "partial", "p", false, gotext.Get("Match partial URL (e.g., github.com instead of full URL)"))

	addFlagCompletion(cmd)
	return cmd
}

func newRepoMirrorClearCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:               "clear",
		Short:             gotext.Get("Remove all mirrors from the repository"),
		Aliases:           []string{"rm-all"},
		Args:              cobra.ExactArgs(1),
		ValidArgsFunction: repoNameCompletion,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := utils.CheckUserPrivileges(); err != nil {
				return err
			}

			name := args[0]

			deps, err := appbuilder.
				New(cmd.Context()).
				WithConfig().
				WithDB().
				WithReposNoPull().
				Build()
			if err != nil {
				return err
			}
			defer deps.Defer()

			reposSlice := deps.Cfg.Repos()
			repoIndex := -1
			urlIndicesToRemove := []int{}

			// Находим репозиторий
			for i, repo := range reposSlice {
				if repo.Name == name {
					repoIndex = i
					break
				}
			}

			if repoIndex == -1 {
				return fmt.Errorf("%s", gotext.Get("Repo \"%s\" does not exist", name))
			}

			reposSlice[repoIndex].Mirrors = []string{}

			deps.Cfg.SetRepos(reposSlice)
			err = deps.Cfg.System.Save()
			if err != nil {
				return fmt.Errorf("%s: %w", gotext.Get("Error saving config"), err)
			}

			if len(urlIndicesToRemove) > 1 {
				fmt.Println(gotext.Get("Removed %d mirrors from repo \"%s\"\n", len(urlIndicesToRemove), name))
			}

			return nil
		},
	}

	addFlagCompletion(cmd)
	return cmd
}

func repoNameCompletion(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	ctx := cmd.Context()
	deps, err := appbuilder.New(ctx).WithConfig().Build()
	if err != nil {
		return nil, cobra.ShellCompDirectiveError
	}
	defer deps.Defer()

	var out []string
	for _, repo := range deps.Cfg.Repos() {
		if toComplete == "" || strings.HasPrefix(repo.Name, toComplete) {
			out = append(out, repo.Name)
		}
	}
	return out, cobra.ShellCompDirectiveNoFileComp
}
