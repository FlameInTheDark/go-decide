# Record the custom questions demo with:
#   go build -o decide.exe ./cmd/decide
#   vhs examples/custom.tape
Output docs/custom.gif

Set Shell "bash"
# Same dimensions as classify.tape so the two demos line up in the README.
Set FontSize 14
Set Width 1100
Set Height 620
Set Theme { "background": "#0f1116", "foreground": "#e6e6e6" }
# See examples/classify.tape for why this is not lower.
Set TypingSpeed 70ms

# Prepare a neutral prompt and a clean screen. The setup is hidden so it never
# appears in the recording.
Hide
Type "export PS1='terminal> '"
Enter
Type "clear"
Enter
Sleep 1s
Show
Sleep 1s

# Ask our own questions instead of the built-in demo set. Every part of a
# question is stated explicitly: there is no built-in rubric behind --scale.
#
# This stays on one line on purpose. A trailing backslash for bash line
# continuation cannot be typed reliably from a tape file: VHS does not unescape
# it, so "\\\\" reaches the shell as two backslashes, bash treats that as an
# escaped backslash and runs every following line as its own command.
Type "./decide.exe classify -o 'bug:Broken software' -o 'billing:Money problems' -o 'account:Login issues' --noul 'refund:Money back requested?' --yes 'Yes, refund' --no 'No refund' --scale 'urgency:How urgent?' --level 'Annoying' --level 'Blocking' 'I cannot reset my password.'"
Enter
# A fixed delay, generous enough for a cold model. Warm the model first, see
# examples/classify.tape for why.
Sleep 20s