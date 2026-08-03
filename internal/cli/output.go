package cli

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"sort"
	"strings"
	"text/tabwriter"
	"time"

	"third-space-cli/internal/thirdspace"
)

func printClubs(output io.Writer, clubs []thirdspace.Club) error {
	table := tabwriter.NewWriter(output, 0, 4, 2, ' ', 0)
	fmt.Fprintln(table, "ID\tNAME\tCODE")
	for _, club := range clubs {
		fmt.Fprintf(table, "%s\t%s\t%s\n", club.ID, club.Name, emptyDash(club.Code))
	}
	return table.Flush()
}

func printClasses(output io.Writer, classes []thirdspace.Class) error {
	if len(classes) == 0 {
		fmt.Fprintln(output, "No classes found.")
		return nil
	}
	table := tabwriter.NewWriter(output, 0, 4, 2, ' ', 0)
	fmt.Fprintln(table, "ID\tSTART (LONDON)\tCLUB / ZONE\tCLASS\tTRAINER\tSPACES\tACTION")
	for _, class := range classes {
		fmt.Fprintf(table, "%s\t%s\t%s\t%s\t%s\t%s\t%s\n",
			class.ID,
			formatClassDate(class.StartDate),
			emptyDash(class.ClubNameWithZone()),
			emptyDash(class.Name),
			emptyDash(class.TrainerName),
			formatValue(class.Spaces),
			emptyDash(class.EventAction),
		)
	}
	return table.Flush()
}

func printClassDetail(output io.Writer, class thirdspace.Class) {
	fmt.Fprintf(output, "Class %s: %s\n", class.ID, emptyDash(class.Name))
	fmt.Fprintf(output, "  Start:   %s\n", formatClassDate(class.StartDate))
	fmt.Fprintf(output, "  Club:    %s\n", emptyDash(class.ClubNameWithZone()))
	fmt.Fprintf(output, "  Trainer: %s\n", emptyDash(class.TrainerName))
	if class.EventAction != "" {
		fmt.Fprintf(output, "  Action:  %s\n", class.EventAction)
	}
	if class.Spaces != nil {
		fmt.Fprintf(output, "  Spaces:  %s\n", formatValue(class.Spaces))
	}
}

func filterClasses(classes []thirdspace.Class, name, trainer string) []thirdspace.Class {
	name = strings.ToLower(strings.TrimSpace(name))
	trainer = strings.ToLower(strings.TrimSpace(trainer))
	filtered := make([]thirdspace.Class, 0, len(classes))
	for _, class := range classes {
		if name != "" && !strings.Contains(strings.ToLower(class.Name), name) {
			continue
		}
		if trainer != "" && !strings.Contains(strings.ToLower(class.TrainerName), trainer) {
			continue
		}
		filtered = append(filtered, class)
	}
	return filtered
}

func sortClasses(classes []thirdspace.Class) {
	sort.SliceStable(classes, func(left, right int) bool {
		leftDate, leftErr := thirdspace.ParseTimestamp(classes[left].StartDate)
		rightDate, rightErr := thirdspace.ParseTimestamp(classes[right].StartDate)
		if leftErr == nil && rightErr == nil {
			return leftDate.Before(rightDate)
		}
		return classes[left].StartDate < classes[right].StartDate
	})
}

func formatClassDate(value string) string {
	parsed, err := thirdspace.ParseTimestamp(value)
	if err != nil {
		return emptyDash(value)
	}
	london, err := time.LoadLocation("Europe/London")
	if err != nil {
		return parsed.Format("Mon 02 Jan 2006 15:04")
	}
	return parsed.In(london).Format("Mon 02 Jan 2006 15:04")
}

func formatValue(value any) string {
	if value == nil {
		return "-"
	}
	switch typed := value.(type) {
	case string:
		return emptyDash(typed)
	case float64:
		if typed == float64(int64(typed)) {
			return fmt.Sprintf("%d", int64(typed))
		}
	}
	return fmt.Sprint(value)
}

func printJSON(output io.Writer, value any) error {
	var data []byte
	var err error
	if raw, ok := value.(json.RawMessage); ok {
		var decoded any
		if err := json.Unmarshal(raw, &decoded); err != nil {
			return fmt.Errorf("decode response JSON: %w", err)
		}
		data, err = json.MarshalIndent(decoded, "", "  ")
	} else {
		data, err = json.MarshalIndent(value, "", "  ")
	}
	if err != nil {
		return fmt.Errorf("encode JSON: %w", err)
	}
	_, err = fmt.Fprintln(output, string(data))
	return err
}

func meaningfulJSON(raw json.RawMessage) bool {
	trimmed := bytes.TrimSpace(raw)
	return len(trimmed) > 0 && !bytes.Equal(trimmed, []byte("null")) && !bytes.Equal(trimmed, []byte("{}"))
}

func splitCommaValues(value string) []string {
	parts := strings.Split(value, ",")
	result := make([]string, 0, len(parts))
	seen := make(map[string]struct{}, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		if _, exists := seen[part]; exists {
			continue
		}
		seen[part] = struct{}{}
		result = append(result, part)
	}
	return result
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}

func emptyDash(value string) string {
	if strings.TrimSpace(value) == "" {
		return "-"
	}
	return value
}
