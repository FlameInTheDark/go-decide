# Record the README demo with:
#   go build -o decide.exe ./cmd/decide
#   vhs examples/classify.tape
#
# Requires a decision model: ollama pull nimble
Output docs/classify.gif

Set Shell "bash"
Set FontSize 16
Set Width 1100
Set Height 560
Set Theme { "background": "#0f1116", "foreground": "#e6e6e6" }
# The GIF runs at 25fps, so one frame every 40ms. 40ms per character lands one
# character per frame and looks instantaneous, while 110ms drags at nearly
# three frames per character. 70ms is the sweet spot: roughly one and a half
# frames per character, so the text visibly appears.
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

Type "./decide.exe 'Our checkout has been returning 500 errors since 9am and customers cannot pay.'"
Enter
# A fixed delay, generous enough for a cold model. Do NOT use Wait here: it
# only tracks the prompt line, so it never sees the command output.
# Warm the model first, e.g. `./decide.exe "warm up" -noul "ok:Is it ok?"`,
# otherwise the first inference has to load the weights and the output below
# would be cut off.
Sleep 20s

Type "clear"
Enter
Sleep 1s

Type "./decide.exe models"
Enter
Sleep 5s