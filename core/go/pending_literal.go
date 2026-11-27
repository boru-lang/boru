package core

// A PENDING container literal is a list or map the parser wrote (Eval, not
// quoted) that nothing has consumed yet: the interpreter evaluates it where a
// word takes it, at the end of the run it is left in, or never (a code-body
// slot takes it raw). These predicates say which residuals the end-of-run
// sweep (autoEvalStack) evaluates.

// isPendingList is a residual list the sweep evaluates as a sub-program.
func isPendingList(v Value) bool {
	return IsEval(&v) && !IsQuoted(&v) && v.Parent().Equal(TList) && v.Data() != nil && !IsTypedList(&v) && !IsTableType(v)
}

// isPendingMap is a residual map the sweep evaluates value by value.
func isPendingMap(v Value) bool {
	return IsEval(&v) && !IsQuoted(&v) && v.Parent().Equal(TMap) && v.Data() != nil && !IsTypedMap(&v) && !IsRecordType(v) && !IsOptionsType(v)
}
