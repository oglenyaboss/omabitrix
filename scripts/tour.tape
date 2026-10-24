# vhs scripts/tour.tape — тур по фичам для README (нужны vhs, ttyd, make build)
Output "docs/assets/tour.gif"

Set Shell "bash"
Set FontFamily "FiraCode Nerd Font"
Set FontSize 16
Set LineHeight 1.15
Set Width 1400
Set Height 820
Set Padding 22
Set Margin 0
Set WindowBar Colorful
Set BorderRadius 10
Set Framerate 30
Set Theme { "name": "omabitrix", "background": "#0d1215", "foreground": "#e6e6e6", "cursor": "#39d3b6", "selection": "#3a4844", "black": "#0d1215", "red": "#f26d7d", "green": "#39d3b6", "yellow": "#e0af68", "blue": "#7aa2f7", "magenta": "#bb9af7", "cyan": "#7dcfff", "white": "#e6e6e6", "brightBlack": "#7a8a86", "brightRed": "#f26d7d", "brightGreen": "#39d3b6", "brightYellow": "#e0af68", "brightBlue": "#7aa2f7", "brightMagenta": "#bb9af7", "brightCyan": "#7dcfff", "brightWhite": "#ffffff" }
Set TypingSpeed 55ms

Hide
Type "clear && ./bin/omabitrix -demo"
Enter
Show
Sleep 5.2s

Type "2"
Sleep 1.5s
Type "j"
Sleep 0.5s
Type "L"
Sleep 1.4s
Screenshot "docs/assets/board.png"
Enter
Sleep 2.2s
Escape
Sleep 0.4s

Type "3"
Sleep 0.6s
Type "g"
Sleep 0.2s
Type "j"
Sleep 0.4s
Enter
Sleep 2.8s
Type "r"
Sleep 0.3s
Type "Гляну пуши до обеда 👌"
Sleep 0.3s
Enter
Sleep 4.5s
Escape
Sleep 0.3s
Escape
Sleep 0.4s

Type "g"
Sleep 0.3s
Enter
Sleep 2.2s
Type "v"
Sleep 0.4s
Type "k"
Sleep 0.3s
Type "k"
Sleep 1s
Screenshot "docs/assets/chat.png"
Enter
Sleep 2.8s
Screenshot "docs/assets/image.png"
Escape
Sleep 0.5s
Escape
Sleep 0.4s

Type "4"
Sleep 2.5s
Screenshot "docs/assets/notify.png"
Sleep 0.5s
