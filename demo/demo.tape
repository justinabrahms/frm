Output demo.gif

Set FontSize 16
Set Width 1200
Set Height 700
Set Theme "Catppuccin Mocha"
Set TypingSpeed 50ms
Set Padding 20

# --- frm list: see all tracked contacts ---

Type "frm list"
Enter
Sleep 2s

# --- frm check: who's overdue? ---

Type "frm check"
Enter
Sleep 2s

# --- frm context: pre-meeting prep with email context ---

Type `frm context "Sarah Chen"`
Enter
Sleep 3s

# --- frm log: record an interaction ---

Type `frm log "Sarah Chen" --note "caught up over coffee"`
Enter
Sleep 2s

# --- frm check: Sarah is no longer overdue ---

Type "frm check"
Enter
Sleep 2s

# --- frm triage: categorize new contacts ---

Type "frm triage # categorize new contacts"
Enter
Sleep 1s

# Kenji Nakamura -> monthly
Type "m"
Enter
Sleep 500ms

# Noor Abbasi -> skip
Type "s"
Enter
Sleep 500ms

# Riley Thompson -> ignore
Type "i"
Enter
Sleep 2s

# --- frm stats: dashboard ---

Type "frm stats"
Enter
Sleep 2s
