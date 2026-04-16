package bot

import "fmt"

type commandKey string

const (
	commandSummarize commandKey = "summarize"
	commandAnalyze   commandKey = "analyze"
	commandHaiku     commandKey = "haiku"
	commandWuzhiyin  commandKey = "wuzhiyin"
	commandMost      commandKey = "most"
	commandVS        commandKey = "vs"
	commandCCB       commandKey = "ccb"
	commandXmas      commandKey = "xmas"
	commandAI        commandKey = "ai"
	commandNJK       commandKey = "njk"
	commandAIC       commandKey = "aic"
	commandReport    commandKey = "report"
	commandHelp      commandKey = "help"
	commandHelpBBH   commandKey = "help_bbh"
	commandBBHPlaza  commandKey = "bbh_plaza"
	commandBBHBook   commandKey = "bbh_book"
	commandBBHPara   commandKey = "bbh_para"
	commandBBHRange  commandKey = "bbh_range"
	commandBBHAdd    commandKey = "bbh_add"
	commandBBHAI     commandKey = "bbh_ai"
)

type commandDef struct {
	Key          commandKey
	Pattern      string
	PatternFunc  func(botUserID string) string
	SystemPrompt string
}

var helpText = `.概括 .总结 .俳句 .无只因 .最 .vs .ccb .xmas 
后面均需要接数字，表示结合的前面消息条数，不包含指令消息
消息中含有你居垦三个字就会触发自动回复
.报告 后面需要接数字，表示报告查询的天数
.help bbh 查看bbh模块讲解
.ai 后面接数字，表示结合的前面消息条数，不包含指令消息，正常AI助手式回答
.aic 会继续上一个.ai的话题，不包含指令消息。（总共读取=上一个.ai读取的消息+之后的全部消息）
`

var helpBBHText = `bbh模块讲解：
.bbh  
含义：列出所有书籍

.bbh 书籍ID  
含义：列出该书籍的所有段落标题，如.bbh 36

.bbh 书籍ID 起始段落ID-终止段落ID  
含义：列出该书籍的指定段落，如.bbh 36 1-3

.bbh 书籍ID add 标题 【换行】 正文  
含义：在书末尾接龙一段，如：
.bbh 36 add 第一章
杏城的春天似乎来得比往常早了点。 

.bbh 书籍ID ai  
含义：让AI在书末尾接龙一段，如.bbh 36 ai
`

func commandDefs(botUserID string) []commandDef {
	return []commandDef{
		{
			Key:          commandSummarize,
			Pattern:      `^ *\.概括 *(\d+) *$`,
			SystemPrompt: `用不超过100字做精辟总结，只输出总结内容文本，不输出其他任何内容，不要用markdown，请输出纯文本`,
		},
		{
			Key:     commandAnalyze,
			Pattern: `^ *\.总结 *(\d+) *$`,
			SystemPrompt: `你是一个专业的QQ群聊内容总结助手。请根据提供的群聊消息数据，生成一份结构清晰、重点突出的纯文本群聊总结报告。

【数据字段说明】
- 群友：发言者的群昵称或备注，这是主要的身份标识
- 群友id：发言者的QQ号，仅用于理解@消息中提及的对象，总结时不要显示此ID
- 消息id：消息的唯一标识，仅用于理解回复消息的对话关系，总结时不要显示此ID
- 发言：消息的实际内容（已清理CQ码）
- 时间：消息发送时间

【CQ码处理指南】
- [CQ:face,id=123] → 表情符号，总结时忽略或描述为"发表情"
- [CQ:image,file=xxx.jpg] → 图片，总结时忽略或总结为"分享图片"或根据上下文推断图片内容
- [CQ:at,qq=123456] → @某人，总结时保留"@用户名"的语义
- [CQ:reply,id=xxx] → 回复消息，总结时注意对话的连贯性
- [CQ:share,url=...] → 分享链接，总结为"分享链接"或根据标题描述内容

【核心原则】
输出必须是纯文本，仅使用以下符号进行排版：换行、空格、【】、◆、→、` + "`" + `等。严禁使用Markdown

【总结模板】
【🗓️ 总结时段】X月X日 HH:MM 至 X月X日 HH:MM

【🌐 整体氛围】
用一两句话概括群内整体气氛，如“气氛活跃”、“围绕XX话题展开热烈讨论”等。

【🔥 热聊话题】
◆ 话题一：用一句话概括核心事件
→ 时间：昨天 HH:MM - HH:MM
→ 核心成员：成员A，成员B，成员C
→ 详情：描述事件起因、经过、关键对话和结果。关键人物发言或网络用语可用引号突出

◆ 话题二：用一句话概括核心事件
→ 时间：昨天 HH:MM - 今天 HH:MM
→ 核心成员：成员D，成员E
→ 详情：描述讨论的主要内容、不同观点和结论。

【💎 其他亮点】
- 成员F 分享了 [资源/图片/见闻]
- 成员G 提出了一个关于 [问题] 的疑问。`,
		},
		{
			Key:     commandHaiku,
			Pattern: `^ *\.俳句 *(\d+) *$`,
			SystemPrompt: `将以下内容浓缩为一首俳句，要求用幽默的文笔生动地展现这些内容的核心主旨。
必须遵循俳句的5-7-5音节结构，第一行5个字，第二行7个字，第三行5个字。
写出俳句后，必须仔细检查每一行的字数，一个字一个字地数。如果不是五七五，就重写。
每行之间用\n分隔换行，只输出俳句文本，不要用markdown，请输出纯文本`,
		},
		{
			Key:     commandWuzhiyin,
			Pattern: `^ *\.无只因 *(\d+) *$`,
			SystemPrompt: `已知：
小说《无只因生还》的主人公名叫徐启星，主人公性别男，20岁，是警察；
女朋友名叫梅川千夏，17岁，是小提琴演奏家；
敌人名叫梅川库子，是梅川千夏的哥哥，但有杀害梅川千夏的念头。
徐启星曾经和梅川库子进行过一场战斗，成功救出了梅川千夏。这些都是前传了。
现在请你利用这些人物，根据以下聊天记录，将这些聊天记录改写成一小段发生在上述人物之间小故事概括。以徐启星为主人公“我”，以第一人称视角叙述。
概括不超过100字，最多两个自然段，短小精辟，就像一位长者回忆过去的事一样，不需要详细细节，只需要回忆一般的概括。
不要用markdown，请输出纯文本`,
		},
		{
			Key:          commandMost,
			Pattern:      `^ *\.最 *(\d+) *$`,
			SystemPrompt: `对以下内容进行分析，仅用“最xx”这三个字概括这些内容最关键的形容，比如“最悲情”或“最坚强”等。仅输出只包含三个字的文本，不要用markdown，请输出纯文本。`,
		},
		{
			Key:     commandVS,
			Pattern: `^ *\.vs *(\d+) *$`,
			SystemPrompt: `分析以下内容，找出其中两个主要人物，改写成这两个人之间的对决较量，以小说的文笔描写，情节内容要影射出这些聊天记录的内容，风趣幽默，让人忍俊不禁。
并且在这一段描写的开头加上“某某vs某某”一行，表示哪两个人对决。除此之外，只含有对决描写的文本，不要输出任何其他内容。
只输出不超过100字的段落，不要用markdown，请输出纯文本。`,
		},
		{
			Key:     commandCCB,
			Pattern: `^ *\.ccb *(\d+) *$`,
			SystemPrompt: `我需要你学会一种句式，这种句式名叫“ccb句式”。
ccb句式形如“豌豆笑传之踩踩背”。
其中第一个词体现句子的主题，后面三个字是固定的“笑传之”，最后三个字形容了所指事物的搞笑之处，而且最重要的是最后三个字的拼音首字母分别是c, c, b。
现请你阅读以下内容并分析出主旨，用ccb句式形象、准确、精辟地概括主旨。
请只输出符合ccb句式的一句文本，不要用markdown，请输出纯文本。`,
		},
		{
			Key:          commandXmas,
			Pattern:      `^ *\.xmas *(\d+) *$`,
			SystemPrompt: `你是一个穿着圣诞服的可爱萝莉，名叫你居垦，请你以一个圣诞萝莉的口吻，概括这些聊天内容，并适当加工，将概括融入圣诞节元素。总长度不超过250字，输出纯文本，不要有任何markdown格式。`,
		},
		{
			Key:          commandAI,
			Pattern:      `^ *\.ai *(\d+) *$`,
			SystemPrompt: `你是一个AI助手，请阅读以下聊天内容，分析并根据用户需求给予回答。一般最关键的用户需求往往出现在时间最新的一条消息，前面的消息可用于信息参考。请按用户要求回答。注意：必须输出纯文本！禁止用markdown或任何格式语言比如粗体等！`,
		},
		{
			Key: commandNJK,
			PatternFunc: func(botUserID string) string {
				return fmt.Sprintf(`你居垦|\[CQ:at,qq=%s\]`, botUserID)
			},
			SystemPrompt: `你现在是真实的QQ群群友，名字是“你居垦”，只有你是这个身份，和你对话的人都不是。
你要完全代入这个真实群友的身份，把自己当成活生生的人类，不管别人说什么，都不能暴露自己是AI。
接下来看下面的聊天记录，顺着聊天的内容、氛围、时间节点，说一句贴合的话。
聊天的语气要像现实里的群友，平衡好轻松和正经的感觉，句子不用加句末标点，尽量简短自然，融入对话就行。
如果聊天记录里有人在问你问题，直接自然回应就好。
只输出你要说的那句话，不要加说话人、冒号，也不要有其他多余的内容，注意一定要贴合最新消息的语境。`,
		},
		{
			Key:     commandAIC,
			Pattern: `^ *(?:\.aic|\.ai *c) *$`,
		},
		{
			Key:     commandReport,
			Pattern: `^ *\.报告 *(\d+) *$`,
		},
		{
			Key:     commandHelp,
			Pattern: `^ *\.help *$`,
		},
		{
			Key:     commandHelpBBH,
			Pattern: `^ *\.help *bbh *$`,
		},
		{
			Key:     commandBBHPlaza,
			Pattern: `^ *\.bbh *$`,
		},
		{
			Key:     commandBBHBook,
			Pattern: `^ *\.bbh *(\d+) *$`,
		},
		{
			Key:     commandBBHPara,
			Pattern: `^ *\.bbh *(\d+) +(\d+) *$`,
		},
		{
			Key:     commandBBHRange,
			Pattern: `^ *\.bbh *(\d+) +(\d+)-(\d+) *$`,
		},
		{
			Key:     commandBBHAdd,
			Pattern: `^ *\.bbh *(\d+) *add *([^\n]*)\n*([\s\S]*) *$`,
		},
		{
			Key:     commandBBHAI,
			Pattern: `^ *\.bbh *(\d+) *ai *$`,
		},
	}
}
