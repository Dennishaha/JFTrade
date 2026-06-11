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
	Name     string
	Version  string
	Symbol   string
	Interval string
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

type OrderStmt struct {
	Range              SourceRange
	Action             OrderAction
	QuantityMode       string
	QuantityExpression string
	EntryPolicy        string
	OrderType          string
	LimitExpression    string
}

func (s *OrderStmt) Kind() StatementKind {
	return StatementKindOrder
}

func (s *OrderStmt) SourceRange() SourceRange {
	return s.Range
}

type ProtectStmt struct {
	Range                SourceRange
	Direction            string
	Mode                 string
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
