//go:build testtools

package modules

// deletionRowOf returns the row with the given code, from either table.
func deletionRowOf(code string) (deletionRow, bool) {
	return deletionFindRow(func(r deletionRow) bool { return r.Code == code })
}

// deletionRowOfKind returns the row of the given kind, from either table.
func deletionRowOfKind(kind string) (deletionRow, bool) {
	return deletionFindRow(func(r deletionRow) bool { return r.Kind == kind })
}

// deletionFindRow returns the first row, service rows before bot rows, that satisfies match.
func deletionFindRow(match func(deletionRow) bool) (deletionRow, bool) {
	for _, table := range [][]deletionRow{deletionServiceRows, deletionBotRows} {
		for _, row := range table {
			if match(row) {
				return row, true
			}
		}
	}
	return deletionRow{}, false
}
