package main

import (
	"bytes"
	"fmt"
	"strconv"
	"strings"

	git2go "github.com/libgit2/git2go/v28"
)

type gitRepo struct {
	working    *gitStatus
	staging    *gitStatus
	ahead      int
	behind     int
	HEAD       string
	upstream   string
	stashCount int
	root       string
	repo       *git2go.Repository
}

type gitStatus struct {
	unmerged  int
	deleted   int
	added     int
	modified  int
	untracked int
	changed   bool
}

func (s *gitStatus) string(prefix, color string) string {
	var status string
	stringIfValue := func(value int, prefix string) string {
		if value > 0 {
			return fmt.Sprintf(" %s%d", prefix, value)
		}
		return ""
	}
	status += stringIfValue(s.added, "+")
	status += stringIfValue(s.modified, "~")
	status += stringIfValue(s.deleted, "-")
	status += stringIfValue(s.untracked, "?")
	status += stringIfValue(s.unmerged, "x")
	if status != "" {
		return fmt.Sprintf("<%s>%s%s</>", color, prefix, status)
	}
	return status
}

type git struct {
	props       *properties
	env         environmentInfo
	repo        *gitRepo
	commandPath string
}

const (
	// BranchIcon the icon to use as branch indicator
	BranchIcon Property = "branch_icon"
	// BranchIdenticalIcon the icon to display when the remote and local branch are identical
	BranchIdenticalIcon Property = "branch_identical_icon"
	// BranchAheadIcon the icon to display when the local branch is ahead of the remote
	BranchAheadIcon Property = "branch_ahead_icon"
	// BranchBehindIcon the icon to display when the local branch is behind the remote
	BranchBehindIcon Property = "branch_behind_icon"
	// BranchGoneIcon the icon to use when ther's no remote
	BranchGoneIcon Property = "branch_gone_icon"
	// LocalWorkingIcon the icon to use as the local working area changes indicator
	LocalWorkingIcon Property = "local_working_icon"
	// LocalStagingIcon the icon to use as the local staging area changes indicator
	LocalStagingIcon Property = "local_staged_icon"
	// DisplayStatus shows the status of the repository
	DisplayStatus Property = "display_status"
	// DisplayStatusDetail shows the detailed status of the repository
	DisplayStatusDetail Property = "display_status_detail"
	// RebaseIcon shows before the rebase context
	RebaseIcon Property = "rebase_icon"
	// CherryPickIcon shows before the cherry-pick context
	CherryPickIcon Property = "cherry_pick_icon"
	// CommitIcon shows before the detached context
	CommitIcon Property = "commit_icon"
	// NoCommitsIcon shows when there are no commits in the repo yet
	NoCommitsIcon Property = "no_commits_icon"
	// TagIcon shows before the tag context
	TagIcon Property = "tag_icon"
	// DisplayStashCount show stash count or not
	DisplayStashCount Property = "display_stash_count"
	// StashCountIcon shows before the stash context
	StashCountIcon Property = "stash_count_icon"
	// StatusSeparatorIcon shows between staging and working area
	StatusSeparatorIcon Property = "status_separator_icon"
	// MergeIcon shows before the merge context
	MergeIcon Property = "merge_icon"
	// DisplayUpstreamIcon show or hide the upstream icon
	DisplayUpstreamIcon Property = "display_upstream_icon"
	// GithubIcon shows√ when upstream is github
	GithubIcon Property = "github_icon"
	// BitbucketIcon shows  when upstream is bitbucket
	BitbucketIcon Property = "bitbucket_icon"
	// GitlabIcon shows when upstream is gitlab
	GitlabIcon Property = "gitlab_icon"
	// GitIcon shows when the upstream can't be identified
	GitIcon Property = "git_icon"
	// WorkingColor if set, the color to use on the working area
	WorkingColor Property = "working_color"
	// StagingColor if set, the color to use on the staging area
	StagingColor Property = "staging_color"
	// StatusColorsEnabled enables status colors
	StatusColorsEnabled Property = "status_colors_enabled"
	// LocalChangesColor if set, the color to use when there are local changes
	LocalChangesColor Property = "local_changes_color"
	// AheadAndBehindColor if set, the color to use when the branch is ahead and behind the remote
	AheadAndBehindColor Property = "ahead_and_behind_color"
	// BehindColor if set, the color to use when the branch is ahead and behind the remote
	BehindColor Property = "behind_color"
	// AheadColor if set, the color to use when the branch is ahead and behind the remote
	AheadColor Property = "ahead_color"
)

func (g *git) enabled() bool {
	commandPath, commandExists := g.env.hasCommand("git")
	if !commandExists {
		return false
	}
	g.commandPath = commandPath
	output, _ := g.env.runCommand(g.commandPath, "rev-parse", "--is-inside-work-tree")
	return output == "true"
}

func (g *git) string() string {
	// initialize repo
	g.repo = &gitRepo{}
	// find repo root
	g.repo.root, _ = git2go.Discover(g.env.getcwd(), false, nil)
	// open repo
	var err error
	g.repo.repo, err = git2go.OpenRepository(g.repo.root)
	if err != nil {
		return ""
	}
	// if bare repo, no status
	if !g.repo.repo.IsBare() {
		g.setGitStatus()
	}

	if g.props.getBool(StatusColorsEnabled, false) {
		g.SetStatusColor()
	}
	buffer := new(bytes.Buffer)
	// branchName
	if g.repo.upstream != "" && g.props.getBool(DisplayUpstreamIcon, false) {
		fmt.Fprintf(buffer, "%s", g.getUpstreamSymbol())
	}
	fmt.Fprintf(buffer, "%s", g.repo.HEAD)
	displayStatus := g.props.getBool(DisplayStatus, true)
	if !displayStatus {
		return buffer.String()
	}
	// if ahead, print with symbol
	if g.repo.ahead > 0 {
		fmt.Fprintf(buffer, " %s%d", g.props.getString(BranchAheadIcon, "\u2191"), g.repo.ahead)
	}
	// if behind, print with symbol
	if g.repo.behind > 0 {
		fmt.Fprintf(buffer, " %s%d", g.props.getString(BranchBehindIcon, "\u2193"), g.repo.behind)
	}
	if g.repo.behind == 0 && g.repo.ahead == 0 && g.repo.upstream != "" {
		fmt.Fprintf(buffer, " %s", g.props.getString(BranchIdenticalIcon, "\u2261"))
	} else if g.repo.upstream == "" {
		fmt.Fprintf(buffer, " %s", g.props.getString(BranchGoneIcon, "\u2262"))
	}
	if g.repo.staging.changed {
		fmt.Fprint(buffer, g.getStatusDetailString(g.repo.staging, StagingColor, LocalStagingIcon, " \uF046"))
	}
	if g.repo.staging.changed && g.repo.working.changed {
		fmt.Fprint(buffer, g.props.getString(StatusSeparatorIcon, " |"))
	}
	if g.repo.working.changed {
		fmt.Fprint(buffer, g.getStatusDetailString(g.repo.working, WorkingColor, LocalWorkingIcon, " \uF044"))
	}
	if g.props.getBool(DisplayStashCount, false) && g.repo.stashCount != 0 {
		fmt.Fprintf(buffer, " %s%d", g.props.getString(StashCountIcon, "\uF692 "), g.repo.stashCount)
	}
	return buffer.String()
}

func (g *git) init(props *properties, env environmentInfo) {
	g.props = props
	g.env = env
}

func (g *git) getStatusDetailString(status *gitStatus, color, icon Property, defaultIcon string) string {
	prefix := g.props.getString(icon, defaultIcon)
	foregroundColor := g.props.getColor(color, g.props.foreground)
	if !g.props.getBool(DisplayStatusDetail, true) {
		return fmt.Sprintf("<%s>%s</>", foregroundColor, prefix)
	}
	return status.string(prefix, foregroundColor)
}

func (g *git) getUpstreamSymbol() string {
	upstream := replaceAllString("/.*", g.repo.upstream, "")
	url := g.getGitCommandOutput("remote", "get-url", upstream)
	if strings.Contains(url, "github") {
		return g.props.getString(GithubIcon, "\uF408 ")
	}
	if strings.Contains(url, "gitlab") {
		return g.props.getString(GitlabIcon, "\uF296 ")
	}
	if strings.Contains(url, "bitbucket") {
		return g.props.getString(BitbucketIcon, "\uF171 ")
	}
	return g.props.getString(GitIcon, "\uE5FB ")
}

func (g *git) setGitStatus() {
	output := g.getGitCommandOutput("status", "-unormal", "--short", "--branch")
	splittedOutput := strings.Split(output, "\n")
	g.repo.working = g.parseGitStats(splittedOutput, true)
	g.repo.staging = g.parseGitStats(splittedOutput, false)
	status := g.parseGitStatusInfo(splittedOutput[0])
	if status["local"] != "" {
		g.repo.ahead, _ = strconv.Atoi(status["ahead"])
		g.repo.behind, _ = strconv.Atoi(status["behind"])
		if status["upstream_status"] != "gone" {
			g.repo.upstream = status["upstream"]
		}
	}
	g.repo.HEAD = g.getGitHEADContext(status["local"])
	g.repo.stashCount = g.getStashContext()
}

func (g *git) SetStatusColor() {
	if g.props.getBool(ColorBackground, true) {
		g.props.background = g.getStatusColor(g.props.background)
	} else {
		g.props.foreground = g.getStatusColor(g.props.foreground)
	}
}

func (g *git) getStatusColor(defaultValue string) string {
	if g.repo.staging.changed || g.repo.working.changed {
		return g.props.getColor(LocalChangesColor, defaultValue)
	} else if g.repo.ahead > 0 && g.repo.behind > 0 {
		return g.props.getColor(AheadAndBehindColor, defaultValue)
	} else if g.repo.ahead > 0 {
		return g.props.getColor(AheadColor, defaultValue)
	} else if g.repo.behind > 0 {
		return g.props.getColor(BehindColor, defaultValue)
	}
	return defaultValue
}

func (g *git) getGitCommandOutput(args ...string) string {
	args = append([]string{"-c", "core.quotepath=false", "-c", "color.status=false"}, args...)
	val, _ := g.env.runCommand(g.commandPath, args...)
	return val
}

func (g *git) getGitHEADContext(ref string) string {
	branchIcon := g.props.getString(BranchIcon, "\uE0A0")
	if ref == "" {
		ref = g.getPrettyHEADName()
	} else {
		ref = fmt.Sprintf("%s%s", branchIcon, ref)
	}
	// rebase
	if g.hasGitFolder("rebase-merge") {
		origin := g.getGitRefFileSymbolicName("rebase-merge/orig-head")
		onto := g.getGitRefFileSymbolicName("rebase-merge/onto")
		step := g.getGitFileContents("rebase-merge/msgnum")
		total := g.getGitFileContents("rebase-merge/end")
		icon := g.props.getString(RebaseIcon, "\uE728 ")
		return fmt.Sprintf("%s%s%s onto %s%s (%s/%s) at %s", icon, branchIcon, origin, branchIcon, onto, step, total, ref)
	}
	if g.hasGitFolder("rebase-apply") {
		head := g.getGitFileContents("rebase-apply/head-name")
		origin := strings.Replace(head, "refs/heads/", "", 1)
		step := g.getGitFileContents("rebase-apply/next")
		total := g.getGitFileContents("rebase-apply/last")
		icon := g.props.getString(RebaseIcon, "\uE728 ")
		return fmt.Sprintf("%s%s%s (%s/%s) at %s", icon, branchIcon, origin, step, total, ref)
	}
	// merge
	if g.hasGitFile("MERGE_HEAD") {
		mergeHEAD := g.getGitRefFileSymbolicName("MERGE_HEAD")
		icon := g.props.getString(MergeIcon, "\uE727 ")
		return fmt.Sprintf("%s%s%s into %s", icon, branchIcon, mergeHEAD, ref)
	}
	// cherry-pick
	if g.hasGitFile("CHERRY_PICK_HEAD") {
		sha := g.getGitRefFileSymbolicName("CHERRY_PICK_HEAD")
		icon := g.props.getString(CherryPickIcon, "\uE29B ")
		return fmt.Sprintf("%s%s onto %s", icon, sha, ref)
	}
	return ref
}

func (g *git) hasGitFile(file string) bool {
	files := fmt.Sprintf(".git/%s", file)
	return g.env.hasFilesInDir(g.repo.root, files)
}

func (g *git) hasGitFolder(folder string) bool {
	path := fmt.Sprintf("%s/.git/%s", g.repo.root, folder)
	return g.env.hasFolder(path)
}

func (g *git) getGitFileContents(file string) string {
	content := g.env.getFileContent(fmt.Sprintf("%s/.git/%s", g.repo.root, file))
	return strings.Trim(content, " \r\n")
}

func (g *git) getGitRefFileSymbolicName(refFile string) string {
	ref := g.getGitFileContents(refFile)
	return g.getGitCommandOutput("name-rev", "--name-only", "--exclude=tags/*", ref)
}

func (g *git) getPrettyHEADName() string {
	// check for tag
	ref := g.getGitCommandOutput("describe", "--tags", "--exact-match")
	if ref != "" {
		return fmt.Sprintf("%s%s", g.props.getString(TagIcon, "\uF412"), ref)
	}
	// fallback to commit
	ref = g.getGitCommandOutput("rev-parse", "--short", "HEAD")
	if ref == "" {
		return g.props.getString(NoCommitsIcon, "\uF594 ")
	}
	return fmt.Sprintf("%s%s", g.props.getString(CommitIcon, "\uF417"), ref)
}

func (g *git) parseGitStats(output []string, working bool) *gitStatus {
	status := gitStatus{}
	if len(output) <= 1 {
		return &status
	}
	for _, line := range output[1:] {
		if len(line) < 2 {
			continue
		}
		code := line[0:1]
		if working {
			code = line[1:2]
		}
		switch code {
		case "?":
			if working {
				status.untracked++
			}
		case "D":
			status.deleted++
		case "A":
			status.added++
		case "U":
			status.unmerged++
		case "M", "R", "C":
			status.modified++
		}
	}
	status.changed = status.added > 0 || status.deleted > 0 || status.modified > 0 || status.unmerged > 0 || status.untracked > 0
	return &status
}

func (g *git) getStashContext() int {
	i := 0
	_ = g.repo.repo.Stashes.Foreach(func(index int, msg string, id *git2go.Oid) error {
		i++
		return nil
	})
	return i
}

func (g *git) parseGitStatusInfo(branchInfo string) map[string]string {
	var branchRegex = `^## (?P<local>\S+?)(\.{3}(?P<upstream>\S+?)( \[(?P<upstream_status>(ahead (?P<ahead>\d+)(, )?)?(behind (?P<behind>\d+))?(gone)?)])?)?$`
	return findNamedRegexMatch(branchRegex, branchInfo)
}
