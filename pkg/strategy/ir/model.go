package ir

type SourceRange struct {
	StartLine int
	EndLine   int
}

type Program struct {
	SourceFormat string
	Metadata     StrategyMetadata
	Hooks        []HookBlock
	Types        []TypeDefinition
	Methods      []MethodDefinition
}

type ObjectField struct {
	Name    string
	Type    string
	Default string
}

type ObjectParameter struct {
	Name    string
	Type    string
	Default string
}

type TypeDefinition struct {
	Range  SourceRange
	Name   string
	Fields []ObjectField
}

type MethodDefinition struct {
	Range        SourceRange
	Name         string
	ReceiverType string
	ReceiverName string
	Parameters   []ObjectParameter
	Body         string
}

type StrategyMetadata struct {
	Name                  string
	Version               string
	Symbol                string
	Interval              string
	DefaultQtyMode        string
	DefaultQtyValue       string
	Pyramiding            int
	InitialCapital        float64
	CommissionType        string
	CommissionValue       float64
	Slippage              int
	ProcessOnClose        bool
	AllowedEntryDirection string
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
	StatementKindLet        StatementKind = "let"
	StatementKindIf         StatementKind = "if"
	StatementKindLog        StatementKind = "log"
	StatementKindNotify     StatementKind = "notify"
	StatementKindOrder      StatementKind = "order"
	StatementKindExit       StatementKind = "exit"
	StatementKindCancel     StatementKind = "cancel"
	StatementKindProtect    StatementKind = "protect"
	StatementKindCollection StatementKind = "collection"
	StatementKindTuple      StatementKind = "tuple"
	StatementKindLoop       StatementKind = "loop"
	StatementKindBreak      StatementKind = "break"
	StatementKindContinue   StatementKind = "continue"
	StatementKindObject     StatementKind = "object"
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

type CollectionStmt struct {
	Range      SourceRange
	Namespace  string
	Operation  string
	Target     string
	ResultName string
	TypeArgs   string
	Arguments  []string
	Mode       AssignmentMode
}

type TupleStmt struct {
	Range       SourceRange
	Names       []string
	Expressions []string
	Mode        AssignmentMode
}

func (s *TupleStmt) Kind() StatementKind      { return StatementKindTuple }
func (s *TupleStmt) SourceRange() SourceRange { return s.Range }

type LoopStmt struct {
	Range           SourceRange
	Variable        string
	IndexVariable   string
	StartExpression string
	EndExpression   string
	StepExpression  string
	WhileCondition  string
	Collection      string
	Body            []Statement
	MaxIterations   int
}

func (s *LoopStmt) Kind() StatementKind      { return StatementKindLoop }
func (s *LoopStmt) SourceRange() SourceRange { return s.Range }

type BreakStmt struct{ Range SourceRange }

func (s *BreakStmt) Kind() StatementKind      { return StatementKindBreak }
func (s *BreakStmt) SourceRange() SourceRange { return s.Range }

type ContinueStmt struct{ Range SourceRange }

func (s *ContinueStmt) Kind() StatementKind      { return StatementKindContinue }
func (s *ContinueStmt) SourceRange() SourceRange { return s.Range }

type ObjectStmt struct {
	Range      SourceRange
	Operation  string
	TypeName   string
	Method     string
	Target     string
	ResultName string
	Arguments  []string
	Mode       AssignmentMode
}

func (s *ObjectStmt) Kind() StatementKind      { return StatementKindObject }
func (s *ObjectStmt) SourceRange() SourceRange { return s.Range }

func (s *CollectionStmt) Kind() StatementKind {
	return StatementKindCollection
}

func (s *CollectionStmt) SourceRange() SourceRange {
	return s.Range
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
	Comment            string
	AlertMessage       string
	DisableAlert       bool
	Immediate          bool
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
	TrailPrice         string
	TrailPoints        string
	TrailOffset        string
	Comment            string
	AlertMessage       string
	DisableAlert       bool
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
	Comment              string
	AlertMessage         string
	DisableAlert         bool
}

func (s *ProtectStmt) Kind() StatementKind {
	return StatementKindProtect
}

func (s *ProtectStmt) SourceRange() SourceRange {
	return s.Range
}
