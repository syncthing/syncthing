package maxminddb

import "reflect"

type verifier struct {
	reader *Reader
}

// Verify checks that the database is valid. It validates the search tree,
// the data section, and the metadata section. This verifier is stricter than
// the specification and may return errors on databases that are readable.
func (r *Reader) Verify() error {
	v := verifier{r}
	if err := v.verifyMetadata(); err != nil {
		return err
	}

	return v.verifyDatabase()
}

func (v *verifier) verifyMetadata() error {
	metadata := v.reader.Metadata

	if metadata.BinaryFormatMajorVersion != 2 {
		return testError(
			"binary_format_major_version",
			2,
			metadata.BinaryFormatMajorVersion,
		)
	}

	if metadata.BinaryFormatMinorVersion != 0 {
		return testError(
			"binary_format_minor_version",
			0,
			metadata.BinaryFormatMinorVersion,
		)
	}

	if metadata.DatabaseType == "" {
		return testError(
			"database_type",
			"non-empty string",
			metadata.DatabaseType,
		)
	}

	if len(metadata.Description) == 0 {
		return testError(
			"description",
			"non-empty slice",
			metadata.Description,
		)
	}

	if metadata.IPVersion != 4 && metadata.IPVersion != 6 {
		return testError(
			"ip_version",
			"4 or 6",
			metadata.IPVersion,
		)
	}

	if metadata.RecordSize != 24 &&
		metadata.RecordSize != 28 &&
		metadata.RecordSize != 32 {
		return testError(
			"record_size",
			"24, 28, or 32",
			metadata.RecordSize,
		)
	}

	if metadata.NodeCount == 0 {
		return testError(
			"node_count",
			"positive integer",
			metadata.NodeCount,
		)
	}
	return nil
}

func (v *verifier) verifyDatabase() error {
	offsets, err := v.verifySearchTree()
	if err != nil {
		return err
	}

	if err := v.verifyDataSectionSeparator(); err != nil {
		return err
	}

	return v.verifyDataSection(offsets)
}

func (v *verifier) verifySearchTree() (map[uint]bool, error) {
	offsets := make(map[uint]bool)

	it := v.reader.Networks()
	for it.Next() {
		offset, err := v.reader.resolveDataPointer(it.lastNode.pointer)
		if err != nil {
			return nil, err
		}
		offsets[uint(offset)] = true
	}
	if err := it.Err(); err != nil {
		return nil, err
	}
	return offsets, nil
}

func (v *verifier) verifyDataSectionSeparator() error {
	separatorStart := v.reader.Metadata.NodeCount * v.reader.Metadata.RecordSize / 4

	separator := v.reader.buffer[separatorStart : separatorStart+dataSectionSeparatorSize]

	for _, b := range separator {
		if b != 0 {
			return newInvalidDatabaseError("unexpected byte in data separator: %v", separator)
		}
	}
	return nil
}

func (v *verifier) verifyDataSection(offsets map[uint]bool) error {
	pointerCount := len(offsets)

	decoder := v.reader.decoder

	var offset uint
	bufferLen := uint(len(decoder.buffer))
	for offset < bufferLen {
		var data interface{}
		rv := reflect.ValueOf(&data)
		newOffset, err := decoder.decode(offset, rv, 0)
		if err != nil {
			return newInvalidDatabaseError("received decoding error (%v) at offset of %v", err, offset)
		}
		if newOffset <= offset {
			return newInvalidDatabaseError("data section offset unexpectedly went from %v to %v", offset, newOffset)
		}

		pointer := offset

		if _, ok := offsets[pointer]; ok {
			delete(offsets, pointer)
		} else {
			return newInvalidDatabaseError("found data (%v) at %v that the search tree does not point to", data, pointer)
		}

		offset = newOffset
	}

	if offset != bufferLen {
		return newInvalidDatabaseError(
			"unexpected data at the end of the data section (last offset: %v, end: %v)",
			offset,
			bufferLen,
		)
	}

	if len(offsets) != 0 {
		return newInvalidDatabaseError(
			"found %v pointers (of %v) in the search tree that we did not see in the data section",
			len(offsets),
			pointerCount,
		)
	}
	return nil
}

func testError(
	field string,
	expected interface{},
	actual interface{},
) error {
	return newInvalidDatabaseError(
		"%v - Expected: %v Actual: %v",
		field,
		expected,
		actual,
	)
}
