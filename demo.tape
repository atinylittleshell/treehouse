Output demo.gif

Require treehouse
Require git

Set Shell "bash"
Set FontSize 16
Set Width 750
Set Height 400
Set Padding 16
Set Theme "rose-pine-moon"

# Suppress update check
Env TREEHOUSE_NO_UPDATE_CHECK "1"
Env SHELL "/bin/bash"

# Setup: create a git repo with a local bare remote, off-screen
Hide
Type "TMPDIR=$(mktemp -d) && git init --bare $TMPDIR/remote.git && git init $TMPDIR/myproject && cd $TMPDIR/myproject && git remote add origin $TMPDIR/remote.git && git commit --allow-empty -m 'init' && git push origin main && clear && cd $TMPDIR"
Enter
Sleep 2s
Show

# Demo: get a worktree
Type "cd myproject"
Enter
Sleep 1s
Type "treehouse"
Enter
Sleep 2s

# Check status
Type "treehouse status"
Enter
Sleep 2s

# Explain: run agent
Type "# run your coding agent and get some work done..."
Sleep 2s
Ctrl+U
Sleep 1s

# Exit subshell (returns worktree to pool)
Type "exit"
Enter
Sleep 2s

# Check status
Type "treehouse status"
Enter
Sleep 2s

# Explain: run agent
Type "# warm worktree preserved for next agent to use"
Sleep 5s

