# Record the JSON file demo with:
#   go build -o decide.exe ./cmd/decide
#   vhs examples/questions.tape
#
# Uses examples/triage.json for the questions and examples/ticket.json for a
# structured state, so the command line stays short.
Output docs/questions.gif

Set Shell "bash"
# Same dimensions as classify.tape so the demos line up in the README.
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

# Just the command and the result. Dumping triage.json here would dominate the
# frame, and the README already shows the file contents.
Type "./decide.exe classify --questions examples/triage.json --state-file examples/ticket.json"
Enter
# A fixed delay, generous enough for a cold model. Warm the model first, see
# examples/classify.tape for why.
Sleep 20s