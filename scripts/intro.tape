# vhs scripts/intro.tape — заставка и пульт для README (нужны vhs, ttyd, make build)
Output "docs/assets/intro.gif"

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

Hide
Type "clear && ./bin/omabitrix -demo"
Enter
Show
Sleep 4.5s
