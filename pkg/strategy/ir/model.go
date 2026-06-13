package ir

type SourceRange struct {
	StartLine int
	EndLine   int
}

type Program struct {
	SourceFormat string
	Metadata     StrategyMetadata
	Hooks        []HookBlock
}

type StrategyMetadata struct {
	Name            string
	Version         string
	Symbol          string
	Interval        string
	DefaultQtyMode  string
	DefaultQtyValue string
	Pyramiding      int
}

type HookKind string

const (
	HookInit       HookKind = "on_init"
	HookKLineClose HookKind = "on_kline_close"
)

type HookBlock struct {
	Kind       HookKind
	Range      SourceRange
	Statements []Statement
}

type StatementKind string

const (
	StatementKindLet     StatementKind = "let"
	StatementKindIf      StatementKind = "if"
	StatementKindLog     StatementKind = "log"
	StatementKindNotify  StatementKind = "notify"
	StatementKindOrder   StatementKind = "order"
	StatementKindExit    StatementKind = "exit"
	StatementKindCancel  StatementKind = "cancel"
	StatementKindProtect StatementKind = "protect"
)

type Statement interface {
	Kind() StatementKind
	SourceRange() SourceRange
}

type LetStmt struct {
	Range      SourceRange
	Name       string
	Expression string
	Mode       AssignmentMode
}

func (s *LetStmt) Kind() StatementKind {
	return StatementKindLet
}

func (s *LetStmt) SourceRange() SourceRange {
	return s.Range
}

type AssignmentMode string

const (
	AssignmentModeLet      AssignmentMode = "let"
	AssignmentModeVar      AssignmentMode = "var"
	AssignmentModeReassign AssignmentMode = "reassign"
)

type IfStmt struct {
	Range     SourceRange
	Condition string
	Then      []Statement
	Else      []Statement
}

func (s *IfStmt) Kind() StatementKind {
	return StatementKindIf
}

func (s *IfStmt) SourceRange() SourceRange {
	return s.Range
}

type LogStmt struct {
	Range   SourceRange
	Message string
}

func (s *LogStmt) Kind() StatementKind {
	return StatementKindLog
}

func (s *LogStmt) SourceRange() SourceRange {
	return s.Range
}

type NotifyStmt struct {
	Range   SourceRange
	Message string
}

func (s *NotifyStmt) Kind() StatementKind {
	return StatementKindNotify
}

func (s *NotifyStmt) SourceRange() SourceRange {
	return s.Range
}

type OrderAction string

const (
	OrderActionBuy   OrderAction = "buy"
	OrderActionSell  OrderAction = "sell"
	OrderActionShort OrderAction = "short"
	OrderActionCover OrderAction = "cover"
)

type OrderIntent string

const (
	OrderIntentEntry   OrderIntent = "entry"
	OrderIntentClose   OrderIntent = "close"
	OrderIntentNet     OrderIntent = "net"
	OrderIntentFlatten OrderIntent = "flatten"
)

type OrderStmt struct {
	Range              SourceRange
	ID                 string
	Action             OrderAction
	Intent             OrderIntent
	QuantityMode       string
	QuantityExpression string
	EntryPolicy        string
	OrderType          string
	LimitExpression    string
	StopExpression     string
}

func (s *OrderStmt) Kind() StatementKind {
	return StatementKindOrder
}

func (s *OrderStmt) SourceRange() SourceRange {
	return s.Range
}

type ExitStmt struct {
	Range              SourceRange
	ID                 string
	FromEntry          string
	Direction          string
	QuantityMode       string
	QuantityExpression string
	StopExpression     string
	LimitExpression    string
	TrailPoints        string
	TrailOffset        string
}

func (s *ExitStmt) Kind() StatementKind {
	return StatementKindExit
}

func (s *ExitStmt) SourceRange() SourceRange {
	return s.Range
}

type CancelStmt struct {
	Range SourceRange
	ID    string
	All   bool
}

func (s *CancelStmt) Kind() StatementKind {
	return StatementKindCancel
}

func (s *CancelStmt) SourceRange() SourceRange {
	return s.Range
}

type ProtectStmt struct {
	Range                SourceRange
	Direction            string
	Mode                 string
	QuantityMode         string
	QuantityExpression   string
	TimeValueExpression  string
	TimeUnit             string
	PercentageExpression string
	WindowPolicy         string
}

func (s *ProtectStmt) Kind() StatementKind {
	return StatementKindProtect
}

func (s *ProtectStmt) SourceRange() SourceRange {
	return s.Range
}
