/**
 * ComposeBoard - Docker Compose 可视化管理面板
 * 行级 Myers 差异算法，用于配置文件变更预览。
 */
const LineDiffUtils = {
    normalizeContent(content) {
        return String(content ?? '').replace(/\r\n?/g, '\n');
    },

    splitLines(content) {
        return this.normalizeContent(content).split('\n');
    },

    buildChanges(oldContent, newContent) {
        const oldLines = this.splitLines(oldContent);
        const newLines = this.splitLines(newContent);
        return this.diffLines(oldLines, newLines).filter(line => line.type !== 'equal');
    },

    diffLines(oldLines, newLines) {
        let prefixLength = 0;
        const sharedLength = Math.min(oldLines.length, newLines.length);
        while (prefixLength < sharedLength && oldLines[prefixLength] === newLines[prefixLength]) {
            prefixLength++;
        }

        let oldSuffixStart = oldLines.length;
        let newSuffixStart = newLines.length;
        while (
            oldSuffixStart > prefixLength &&
            newSuffixStart > prefixLength &&
            oldLines[oldSuffixStart - 1] === newLines[newSuffixStart - 1]
        ) {
            oldSuffixStart--;
            newSuffixStart--;
        }

        const result = [];
        for (let i = 0; i < prefixLength; i++) {
            result.push({ type: 'equal', text: oldLines[i] });
        }

        result.push(...this.diffMiddle(
            oldLines.slice(prefixLength, oldSuffixStart),
            newLines.slice(prefixLength, newSuffixStart)
        ));

        for (let i = oldSuffixStart; i < oldLines.length; i++) {
            result.push({ type: 'equal', text: oldLines[i] });
        }
        return result;
    },

    diffMiddle(oldLines, newLines) {
        if (oldLines.length === 0) {
            return newLines.map(text => ({ type: 'add', text }));
        }
        if (newLines.length === 0) {
            return oldLines.map(text => ({ type: 'remove', text }));
        }

        const maxDistance = oldLines.length + newLines.length;
        const trace = [];
        const frontier = new Map([[1, 0]]);

        for (let distance = 0; distance <= maxDistance; distance++) {
            trace.push(new Map(frontier));
            for (let diagonal = -distance; diagonal <= distance; diagonal += 2) {
                const previousDiagonal = frontier.get(diagonal - 1);
                const nextDiagonal = frontier.get(diagonal + 1);
                let oldIndex;

                if (
                    diagonal === -distance ||
                    (diagonal !== distance && (previousDiagonal ?? -1) < (nextDiagonal ?? -1))
                ) {
                    oldIndex = nextDiagonal ?? 0;
                } else {
                    oldIndex = (previousDiagonal ?? 0) + 1;
                }

                let newIndex = oldIndex - diagonal;
                while (
                    oldIndex < oldLines.length &&
                    newIndex < newLines.length &&
                    oldLines[oldIndex] === newLines[newIndex]
                ) {
                    oldIndex++;
                    newIndex++;
                }
                frontier.set(diagonal, oldIndex);

                if (oldIndex >= oldLines.length && newIndex >= newLines.length) {
                    return this.backtrack(trace, oldLines, newLines);
                }
            }
        }

        return [
            ...oldLines.map(text => ({ type: 'remove', text })),
            ...newLines.map(text => ({ type: 'add', text }))
        ];
    },

    backtrack(trace, oldLines, newLines) {
        let oldIndex = oldLines.length;
        let newIndex = newLines.length;
        const result = [];

        for (let distance = trace.length - 1; distance >= 0; distance--) {
            const frontier = trace[distance];
            const diagonal = oldIndex - newIndex;
            const previousDiagonal = frontier.get(diagonal - 1);
            const nextDiagonal = frontier.get(diagonal + 1);
            const sourceDiagonal = (
                diagonal === -distance ||
                (diagonal !== distance && (previousDiagonal ?? -1) < (nextDiagonal ?? -1))
            ) ? diagonal + 1 : diagonal - 1;
            const previousOldIndex = frontier.get(sourceDiagonal) ?? 0;
            const previousNewIndex = previousOldIndex - sourceDiagonal;

            while (oldIndex > previousOldIndex && newIndex > previousNewIndex) {
                result.push({ type: 'equal', text: oldLines[oldIndex - 1] });
                oldIndex--;
                newIndex--;
            }

            if (distance === 0) {
                break;
            }
            if (oldIndex === previousOldIndex) {
                result.push({ type: 'add', text: newLines[newIndex - 1] });
                newIndex--;
            } else {
                result.push({ type: 'remove', text: oldLines[oldIndex - 1] });
                oldIndex--;
            }
        }

        return result.reverse();
    }
};
