package vmdk

import "io"

type extent interface {
	Size() uint64
	ReadAt(p []byte, off int64) (int, error)
	Close() error
}

type extentSpan struct {
	start uint64 // inclusive (virtual)
	end   uint64 // exclusive (virtual)
	ext   extent
}

// readAcrossSpans читает виртуальные байты через набор extents.
func readAcrossSpans(spans []extentSpan, p []byte, off int64, totalSize uint64) (int, error) {
	if off < 0 {
		return 0, io.ErrUnexpectedEOF
	}
	if uint64(off) >= totalSize {
		return 0, io.EOF
	}

	origLen := len(p)

	read := 0
	curOff := uint64(off)

	// Если запрос выходит за границу образа, читаем только доступную часть.
	max := totalSize - uint64(off)
	if uint64(len(p)) > max {
		p = p[:max]
	}

	for read < len(p) && curOff < totalSize {
		// найти span
		var s *extentSpan
		for i := range spans {
			if curOff >= spans[i].start && curOff < spans[i].end {
				s = &spans[i]
				break
			}
		}
		if s == nil {
			// “дырка” в описании extents — считаем нулями
			want := minU64(uint64(len(p)-read), totalSize-curOff)
			zeroFill(p[read : read+int(want)])
			read += int(want)
			curOff += want
			continue
		}

		within := curOff - s.start
		maxInSpan := s.end - curOff
		want := minU64(uint64(len(p)-read), maxInSpan)

		n, err := s.ext.ReadAt(p[read:read+int(want)], int64(within))
		read += n
		curOff += uint64(n)

		if err != nil {
			// Если underlying extent уже что-то прочитал, а потом вернул EOF
			// на точной границе нашего запроса, это не ошибка.
			if err == io.EOF && read == len(p) {
				break
			}
			return read, err
		}
		if n == 0 {
			// защита от зависания
			return read, io.EOF
		}
	}

	// Контракт ReaderAt:
	// - если запрошенный буфер заполнен полностью, вернуть nil
	// - io.EOF только если запрос пришлось усечь концом образа
	if len(p) == origLen {
		return read, nil
	}
	return read, io.EOF
}
