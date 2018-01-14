//DO NOT EDIT : this file was automatically generated.
package spin

// Spinner defines a spinner widget
type Spinner struct {
	Name     Name
	Interval int
	Frames   []string
}

// Name  represents a name for a spinner item.
type Name uint

// available spinners
const (
	Unknown Name = iota
	Arc
	Arrow
	Arrow2
	Arrow3
	Balloon
	Balloon2
	Bounce
	BouncingBall
	BouncingBar
	BoxBounce
	BoxBounce2
	Christmas
	Circle
	CircleHalves
	CircleQuarters
	Clock
	Dots
	Dots10
	Dots11
	Dots12
	Dots2
	Dots3
	Dots4
	Dots5
	Dots6
	Dots7
	Dots8
	Dots9
	Dqpb
	Earth
	Flip
	GrowHorizontal
	GrowVertical
	Hamburger
	Hearts
	Line
	Line2
	Monkey
	Moon
	Noise
	Pipe
	Pong
	Runner
	Shark
	SimpleDots
	SimpleDotsScrolling
	Smiley
	SquareCorners
	Squish
	Star
	Star2
	Toggle
	Toggle10
	Toggle11
	Toggle12
	Toggle13
	Toggle2
	Toggle3
	Toggle4
	Toggle5
	Toggle6
	Toggle7
	Toggle8
	Toggle9
	Triangle
	Weather
)

func (s Name) String() string {
	switch s {
	case Arc:
		return "arc"
	case Arrow:
		return "arrow"
	case Arrow2:
		return "arrow2"
	case Arrow3:
		return "arrow3"
	case Balloon:
		return "balloon"
	case Balloon2:
		return "balloon2"
	case Bounce:
		return "bounce"
	case BouncingBall:
		return "bouncingBall"
	case BouncingBar:
		return "bouncingBar"
	case BoxBounce:
		return "boxBounce"
	case BoxBounce2:
		return "boxBounce2"
	case Christmas:
		return "christmas"
	case Circle:
		return "circle"
	case CircleHalves:
		return "circleHalves"
	case CircleQuarters:
		return "circleQuarters"
	case Clock:
		return "clock"
	case Dots:
		return "dots"
	case Dots10:
		return "dots10"
	case Dots11:
		return "dots11"
	case Dots12:
		return "dots12"
	case Dots2:
		return "dots2"
	case Dots3:
		return "dots3"
	case Dots4:
		return "dots4"
	case Dots5:
		return "dots5"
	case Dots6:
		return "dots6"
	case Dots7:
		return "dots7"
	case Dots8:
		return "dots8"
	case Dots9:
		return "dots9"
	case Dqpb:
		return "dqpb"
	case Earth:
		return "earth"
	case Flip:
		return "flip"
	case GrowHorizontal:
		return "growHorizontal"
	case GrowVertical:
		return "growVertical"
	case Hamburger:
		return "hamburger"
	case Hearts:
		return "hearts"
	case Line:
		return "line"
	case Line2:
		return "line2"
	case Monkey:
		return "monkey"
	case Moon:
		return "moon"
	case Noise:
		return "noise"
	case Pipe:
		return "pipe"
	case Pong:
		return "pong"
	case Runner:
		return "runner"
	case Shark:
		return "shark"
	case SimpleDots:
		return "simpleDots"
	case SimpleDotsScrolling:
		return "simpleDotsScrolling"
	case Smiley:
		return "smiley"
	case SquareCorners:
		return "squareCorners"
	case Squish:
		return "squish"
	case Star:
		return "star"
	case Star2:
		return "star2"
	case Toggle:
		return "toggle"
	case Toggle10:
		return "toggle10"
	case Toggle11:
		return "toggle11"
	case Toggle12:
		return "toggle12"
	case Toggle13:
		return "toggle13"
	case Toggle2:
		return "toggle2"
	case Toggle3:
		return "toggle3"
	case Toggle4:
		return "toggle4"
	case Toggle5:
		return "toggle5"
	case Toggle6:
		return "toggle6"
	case Toggle7:
		return "toggle7"
	case Toggle8:
		return "toggle8"
	case Toggle9:
		return "toggle9"
	case Triangle:
		return "triangle"
	case Weather:
		return "weather"
	default:
		return ""
	}
}

func Get(name Name) Spinner {
	switch name {
	case Arc:
		return Spinner{
			Name:     Arc,
			Interval: 100,
			Frames:   []string{`◜`, `◠`, `◝`, `◞`, `◡`, `◟`},
		}
	case Arrow:
		return Spinner{
			Name:     Arrow,
			Interval: 100,
			Frames:   []string{`←`, `↖`, `↑`, `↗`, `→`, `↘`, `↓`, `↙`},
		}
	case Arrow2:
		return Spinner{
			Name:     Arrow2,
			Interval: 80,
			Frames:   []string{`⬆️ `, `↗️ `, `➡️ `, `↘️ `, `⬇️ `, `↙️ `, `⬅️ `, `↖️ `},
		}
	case Arrow3:
		return Spinner{
			Name:     Arrow3,
			Interval: 120,
			Frames:   []string{`▹▹▹▹▹`, `▸▹▹▹▹`, `▹▸▹▹▹`, `▹▹▸▹▹`, `▹▹▹▸▹`, `▹▹▹▹▸`},
		}
	case Balloon:
		return Spinner{
			Name:     Balloon,
			Interval: 140,
			Frames:   []string{` `, `.`, `o`, `O`, `@`, `*`, ` `},
		}
	case Balloon2:
		return Spinner{
			Name:     Balloon2,
			Interval: 120,
			Frames:   []string{`.`, `o`, `O`, `°`, `O`, `o`, `.`},
		}
	case Bounce:
		return Spinner{
			Name:     Bounce,
			Interval: 120,
			Frames:   []string{`⠁`, `⠂`, `⠄`, `⠂`},
		}
	case BouncingBall:
		return Spinner{
			Name:     BouncingBall,
			Interval: 80,
			Frames:   []string{`( ●    )`, `(  ●   )`, `(   ●  )`, `(    ● )`, `(     ●)`, `(    ● )`, `(   ●  )`, `(  ●   )`, `( ●    )`, `(●     )`},
		}
	case BouncingBar:
		return Spinner{
			Name:     BouncingBar,
			Interval: 80,
			Frames:   []string{`[    ]`, `[=   ]`, `[==  ]`, `[=== ]`, `[ ===]`, `[  ==]`, `[   =]`, `[    ]`, `[   =]`, `[  ==]`, `[ ===]`, `[====]`, `[=== ]`, `[==  ]`, `[=   ]`},
		}
	case BoxBounce:
		return Spinner{
			Name:     BoxBounce,
			Interval: 120,
			Frames:   []string{`▖`, `▘`, `▝`, `▗`},
		}
	case BoxBounce2:
		return Spinner{
			Name:     BoxBounce2,
			Interval: 100,
			Frames:   []string{`▌`, `▀`, `▐`, `▄`},
		}
	case Christmas:
		return Spinner{
			Name:     Christmas,
			Interval: 400,
			Frames:   []string{`🌲`, `🎄`},
		}
	case Circle:
		return Spinner{
			Name:     Circle,
			Interval: 120,
			Frames:   []string{`◡`, `⊙`, `◠`},
		}
	case CircleHalves:
		return Spinner{
			Name:     CircleHalves,
			Interval: 50,
			Frames:   []string{`◐`, `◓`, `◑`, `◒`},
		}
	case CircleQuarters:
		return Spinner{
			Name:     CircleQuarters,
			Interval: 120,
			Frames:   []string{`◴`, `◷`, `◶`, `◵`},
		}
	case Clock:
		return Spinner{
			Name:     Clock,
			Interval: 100,
			Frames:   []string{`🕐 `, `🕑 `, `🕒 `, `🕓 `, `🕔 `, `🕕 `, `🕖 `, `🕗 `, `🕘 `, `🕙 `, `🕚 `},
		}
	case Dots:
		return Spinner{
			Name:     Dots,
			Interval: 80,
			Frames:   []string{`⠋`, `⠙`, `⠹`, `⠸`, `⠼`, `⠴`, `⠦`, `⠧`, `⠇`, `⠏`},
		}
	case Dots10:
		return Spinner{
			Name:     Dots10,
			Interval: 80,
			Frames:   []string{`⢄`, `⢂`, `⢁`, `⡁`, `⡈`, `⡐`, `⡠`},
		}
	case Dots11:
		return Spinner{
			Name:     Dots11,
			Interval: 100,
			Frames:   []string{`⠁`, `⠂`, `⠄`, `⡀`, `⢀`, `⠠`, `⠐`, `⠈`},
		}
	case Dots12:
		return Spinner{
			Name:     Dots12,
			Interval: 80,
			Frames:   []string{`⢀⠀`, `⡀⠀`, `⠄⠀`, `⢂⠀`, `⡂⠀`, `⠅⠀`, `⢃⠀`, `⡃⠀`, `⠍⠀`, `⢋⠀`, `⡋⠀`, `⠍⠁`, `⢋⠁`, `⡋⠁`, `⠍⠉`, `⠋⠉`, `⠋⠉`, `⠉⠙`, `⠉⠙`, `⠉⠩`, `⠈⢙`, `⠈⡙`, `⢈⠩`, `⡀⢙`, `⠄⡙`, `⢂⠩`, `⡂⢘`, `⠅⡘`, `⢃⠨`, `⡃⢐`, `⠍⡐`, `⢋⠠`, `⡋⢀`, `⠍⡁`, `⢋⠁`, `⡋⠁`, `⠍⠉`, `⠋⠉`, `⠋⠉`, `⠉⠙`, `⠉⠙`, `⠉⠩`, `⠈⢙`, `⠈⡙`, `⠈⠩`, `⠀⢙`, `⠀⡙`, `⠀⠩`, `⠀⢘`, `⠀⡘`, `⠀⠨`, `⠀⢐`, `⠀⡐`, `⠀⠠`, `⠀⢀`, `⠀⡀`},
		}
	case Dots2:
		return Spinner{
			Name:     Dots2,
			Interval: 80,
			Frames:   []string{`⣾`, `⣽`, `⣻`, `⢿`, `⡿`, `⣟`, `⣯`, `⣷`},
		}
	case Dots3:
		return Spinner{
			Name:     Dots3,
			Interval: 80,
			Frames:   []string{`⠋`, `⠙`, `⠚`, `⠞`, `⠖`, `⠦`, `⠴`, `⠲`, `⠳`, `⠓`},
		}
	case Dots4:
		return Spinner{
			Name:     Dots4,
			Interval: 80,
			Frames:   []string{`⠄`, `⠆`, `⠇`, `⠋`, `⠙`, `⠸`, `⠰`, `⠠`, `⠰`, `⠸`, `⠙`, `⠋`, `⠇`, `⠆`},
		}
	case Dots5:
		return Spinner{
			Name:     Dots5,
			Interval: 80,
			Frames:   []string{`⠋`, `⠙`, `⠚`, `⠒`, `⠂`, `⠂`, `⠒`, `⠲`, `⠴`, `⠦`, `⠖`, `⠒`, `⠐`, `⠐`, `⠒`, `⠓`, `⠋`},
		}
	case Dots6:
		return Spinner{
			Name:     Dots6,
			Interval: 80,
			Frames:   []string{`⠁`, `⠉`, `⠙`, `⠚`, `⠒`, `⠂`, `⠂`, `⠒`, `⠲`, `⠴`, `⠤`, `⠄`, `⠄`, `⠤`, `⠴`, `⠲`, `⠒`, `⠂`, `⠂`, `⠒`, `⠚`, `⠙`, `⠉`, `⠁`},
		}
	case Dots7:
		return Spinner{
			Name:     Dots7,
			Interval: 80,
			Frames:   []string{`⠈`, `⠉`, `⠋`, `⠓`, `⠒`, `⠐`, `⠐`, `⠒`, `⠖`, `⠦`, `⠤`, `⠠`, `⠠`, `⠤`, `⠦`, `⠖`, `⠒`, `⠐`, `⠐`, `⠒`, `⠓`, `⠋`, `⠉`, `⠈`},
		}
	case Dots8:
		return Spinner{
			Name:     Dots8,
			Interval: 80,
			Frames:   []string{`⠁`, `⠁`, `⠉`, `⠙`, `⠚`, `⠒`, `⠂`, `⠂`, `⠒`, `⠲`, `⠴`, `⠤`, `⠄`, `⠄`, `⠤`, `⠠`, `⠠`, `⠤`, `⠦`, `⠖`, `⠒`, `⠐`, `⠐`, `⠒`, `⠓`, `⠋`, `⠉`, `⠈`, `⠈`},
		}
	case Dots9:
		return Spinner{
			Name:     Dots9,
			Interval: 80,
			Frames:   []string{`⢹`, `⢺`, `⢼`, `⣸`, `⣇`, `⡧`, `⡗`, `⡏`},
		}
	case Dqpb:
		return Spinner{
			Name:     Dqpb,
			Interval: 100,
			Frames:   []string{`d`, `q`, `p`, `b`},
		}
	case Earth:
		return Spinner{
			Name:     Earth,
			Interval: 180,
			Frames:   []string{`🌍 `, `🌎 `, `🌏 `},
		}
	case Flip:
		return Spinner{
			Name:     Flip,
			Interval: 70,
			Frames:   []string{`_`, `_`, `_`, `-`, "`", "`", `'`, `´`, `-`, `_`, `_`, `_`},
		}
	case GrowHorizontal:
		return Spinner{
			Name:     GrowHorizontal,
			Interval: 120,
			Frames:   []string{`▏`, `▎`, `▍`, `▌`, `▋`, `▊`, `▉`, `▊`, `▋`, `▌`, `▍`, `▎`},
		}
	case GrowVertical:
		return Spinner{
			Name:     GrowVertical,
			Interval: 120,
			Frames:   []string{`▁`, `▃`, `▄`, `▅`, `▆`, `▇`, `▆`, `▅`, `▄`, `▃`},
		}
	case Hamburger:
		return Spinner{
			Name:     Hamburger,
			Interval: 100,
			Frames:   []string{`☱`, `☲`, `☴`},
		}
	case Hearts:
		return Spinner{
			Name:     Hearts,
			Interval: 100,
			Frames:   []string{`💛 `, `💙 `, `💜 `, `💚 `, `❤️ `},
		}
	case Line:
		return Spinner{
			Name:     Line,
			Interval: 130,
			Frames:   []string{`-`, `\`, `|`, `/`},
		}
	case Line2:
		return Spinner{
			Name:     Line2,
			Interval: 100,
			Frames:   []string{`⠂`, `-`, `–`, `—`, `–`, `-`},
		}
	case Monkey:
		return Spinner{
			Name:     Monkey,
			Interval: 300,
			Frames:   []string{`🙈 `, `🙈 `, `🙉 `, `🙊 `},
		}
	case Moon:
		return Spinner{
			Name:     Moon,
			Interval: 80,
			Frames:   []string{`🌑 `, `🌒 `, `🌓 `, `🌔 `, `🌕 `, `🌖 `, `🌗 `, `🌘 `},
		}
	case Noise:
		return Spinner{
			Name:     Noise,
			Interval: 100,
			Frames:   []string{`▓`, `▒`, `░`},
		}
	case Pipe:
		return Spinner{
			Name:     Pipe,
			Interval: 100,
			Frames:   []string{`┤`, `┘`, `┴`, `└`, `├`, `┌`, `┬`, `┐`},
		}
	case Pong:
		return Spinner{
			Name:     Pong,
			Interval: 80,
			Frames:   []string{`▐⠂       ▌`, `▐⠈       ▌`, `▐ ⠂      ▌`, `▐ ⠠      ▌`, `▐  ⡀     ▌`, `▐  ⠠     ▌`, `▐   ⠂    ▌`, `▐   ⠈    ▌`, `▐    ⠂   ▌`, `▐    ⠠   ▌`, `▐     ⡀  ▌`, `▐     ⠠  ▌`, `▐      ⠂ ▌`, `▐      ⠈ ▌`, `▐       ⠂▌`, `▐       ⠠▌`, `▐       ⡀▌`, `▐      ⠠ ▌`, `▐      ⠂ ▌`, `▐     ⠈  ▌`, `▐     ⠂  ▌`, `▐    ⠠   ▌`, `▐    ⡀   ▌`, `▐   ⠠    ▌`, `▐   ⠂    ▌`, `▐  ⠈     ▌`, `▐  ⠂     ▌`, `▐ ⠠      ▌`, `▐ ⡀      ▌`, `▐⠠       ▌`},
		}
	case Runner:
		return Spinner{
			Name:     Runner,
			Interval: 140,
			Frames:   []string{`🚶 `, `🏃 `},
		}
	case Shark:
		return Spinner{
			Name:     Shark,
			Interval: 120,
			Frames:   []string{`▐|\____________▌`, `▐_|\___________▌`, `▐__|\__________▌`, `▐___|\_________▌`, `▐____|\________▌`, `▐_____|\_______▌`, `▐______|\______▌`, `▐_______|\_____▌`, `▐________|\____▌`, `▐_________|\___▌`, `▐__________|\__▌`, `▐___________|\_▌`, `▐____________|\▌`, `▐____________/|▌`, `▐___________/|_▌`, `▐__________/|__▌`, `▐_________/|___▌`, `▐________/|____▌`, `▐_______/|_____▌`, `▐______/|______▌`, `▐_____/|_______▌`, `▐____/|________▌`, `▐___/|_________▌`, `▐__/|__________▌`, `▐_/|___________▌`, `▐/|____________▌`},
		}
	case SimpleDots:
		return Spinner{
			Name:     SimpleDots,
			Interval: 400,
			Frames:   []string{`.  `, `.. `, `...`, `   `},
		}
	case SimpleDotsScrolling:
		return Spinner{
			Name:     SimpleDotsScrolling,
			Interval: 200,
			Frames:   []string{`.  `, `.. `, `...`, ` ..`, `  .`, `   `},
		}
	case Smiley:
		return Spinner{
			Name:     Smiley,
			Interval: 200,
			Frames:   []string{`😄 `, `😝 `},
		}
	case SquareCorners:
		return Spinner{
			Name:     SquareCorners,
			Interval: 180,
			Frames:   []string{`◰`, `◳`, `◲`, `◱`},
		}
	case Squish:
		return Spinner{
			Name:     Squish,
			Interval: 100,
			Frames:   []string{`╫`, `╪`},
		}
	case Star:
		return Spinner{
			Name:     Star,
			Interval: 70,
			Frames:   []string{`✶`, `✸`, `✹`, `✺`, `✹`, `✷`},
		}
	case Star2:
		return Spinner{
			Name:     Star2,
			Interval: 80,
			Frames:   []string{`+`, `x`, `*`},
		}
	case Toggle:
		return Spinner{
			Name:     Toggle,
			Interval: 250,
			Frames:   []string{`⊶`, `⊷`},
		}
	case Toggle10:
		return Spinner{
			Name:     Toggle10,
			Interval: 100,
			Frames:   []string{`㊂`, `㊀`, `㊁`},
		}
	case Toggle11:
		return Spinner{
			Name:     Toggle11,
			Interval: 50,
			Frames:   []string{`⧇`, `⧆`},
		}
	case Toggle12:
		return Spinner{
			Name:     Toggle12,
			Interval: 120,
			Frames:   []string{`☗`, `☖`},
		}
	case Toggle13:
		return Spinner{
			Name:     Toggle13,
			Interval: 80,
			Frames:   []string{`=`, `*`, `-`},
		}
	case Toggle2:
		return Spinner{
			Name:     Toggle2,
			Interval: 80,
			Frames:   []string{`▫`, `▪`},
		}
	case Toggle3:
		return Spinner{
			Name:     Toggle3,
			Interval: 120,
			Frames:   []string{`□`, `■`},
		}
	case Toggle4:
		return Spinner{
			Name:     Toggle4,
			Interval: 100,
			Frames:   []string{`■`, `□`, `▪`, `▫`},
		}
	case Toggle5:
		return Spinner{
			Name:     Toggle5,
			Interval: 100,
			Frames:   []string{`▮`, `▯`},
		}
	case Toggle6:
		return Spinner{
			Name:     Toggle6,
			Interval: 300,
			Frames:   []string{`ဝ`, `၀`},
		}
	case Toggle7:
		return Spinner{
			Name:     Toggle7,
			Interval: 80,
			Frames:   []string{`⦾`, `⦿`},
		}
	case Toggle8:
		return Spinner{
			Name:     Toggle8,
			Interval: 100,
			Frames:   []string{`◍`, `◌`},
		}
	case Toggle9:
		return Spinner{
			Name:     Toggle9,
			Interval: 100,
			Frames:   []string{`◉`, `◎`},
		}
	case Triangle:
		return Spinner{
			Name:     Triangle,
			Interval: 50,
			Frames:   []string{`◢`, `◣`, `◤`, `◥`},
		}
	case Weather:
		return Spinner{
			Name:     Weather,
			Interval: 100,
			Frames:   []string{`☀️ `, `☀️ `, `☀️ `, `🌤 `, `⛅️ `, `🌥 `, `☁️ `, `🌧 `, `🌨 `, `🌧 `, `🌨 `, `🌧 `, `🌨 `, `⛈ `, `🌨 `, `🌧 `, `🌨 `, `☁️ `, `🌥 `, `⛅️ `, `🌤 `, `☀️ `, `☀️ `},
		}
	default:
		return Spinner{}
	}
}
