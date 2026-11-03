package collectionfile

// Sample is one scalar value before a column kind is chosen.
// Null values do not vote on the column type. Text values stay strings,
// which is how quoted CSV cells and joined repeated XML values are kept.
type Sample struct {
	Raw  string
	Null bool
	Text bool
}

func sampleCell(s Sample) cell {
	if s.Null {
		return cell{skip: true, value: nil}
	}
	if s.Text {
		return cell{raw: s.Raw, quoted: true, kind: KindString, value: s.Raw}
	}
	return classifyCell(s.Raw, false)
}

// ColumnKind infers one column from every sample. Values past the first
// sample window can still widen the column. Integer and decimal widen to
// decimal. Date and datetime widen to datetime. Any other mix is string.
func ColumnKind(samples []Sample) Kind {
	cells := make([]cell, len(samples))
	for i, s := range samples {
		cells[i] = sampleCell(s)
	}
	return widenKind(inferColumnKind(cells, DefaultSampleLimit), cells)
}

// Finalize applies the column kind to one sample.
func Finalize(kind Kind, s Sample) any {
	return finalizeCell(kind, sampleCell(s))
}
