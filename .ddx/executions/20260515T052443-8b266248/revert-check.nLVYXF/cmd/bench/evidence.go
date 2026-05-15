package main

import (
	"fmt"
	"os"
	"strings"

	"github.com/easel/fizeau/internal/benchmark/evidence"
)

func cmdEvidence(args []string) int {
	if len(args) == 0 {
		fmt.Fprintf(os.Stderr, "Usage: %s evidence <subcommand>\n\nSubcommands:\n  validate            Validate evidence records in a JSON/JSONL file\n  append              Append validated evidence records to a JSONL ledger\n  import-terminalbench Import a TerminalBench matrix directory into JSONL evidence\n  import-beadbench     Import a beadbench report.json into JSONL evidence\n  import-external      Import curated external benchmark snapshots into JSONL evidence\n", benchCommandName())
		return 2
	}

	switch args[0] {
	case "validate":
		return cmdEvidenceValidate(args[1:])
	case "append":
		return cmdEvidenceAppend(args[1:])
	case "import-terminalbench":
		return cmdEvidenceImportTerminalBench(args[1:])
	case "import-beadbench":
		return cmdEvidenceImportBeadBench(args[1:])
	case "import-external":
		return cmdEvidenceImportExternal(args[1:])
	case "help", "-h", "--help":
		fmt.Fprintf(os.Stderr, "Usage: %s evidence validate <file>\n       %s evidence append --in <records.jsonl> --ledger <ledger.jsonl>\n       %s evidence import-terminalbench --matrix <dir> --out <records.jsonl>\n       %s evidence import-beadbench --report <report.json> --out <records.jsonl>\n       %s evidence import-external --source <fixture> --out <records.jsonl>\n", benchCommandName(), benchCommandName(), benchCommandName(), benchCommandName(), benchCommandName())
		return 0
	default:
		fmt.Fprintf(os.Stderr, "%s evidence: unknown subcommand %q\n", benchCommandName(), args[0])
		return 2
	}
}

func cmdEvidenceValidate(args []string) int {
	fs := flagSet("evidence validate")
	workDir := fs.String("work-dir", "", "Repository root (default: cwd)")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if fs.NArg() != 1 {
		fmt.Fprintf(os.Stderr, "Usage: %s evidence validate <file>\n", benchCommandName())
		return 2
	}

	validator, err := evidence.NewValidator(resolveWorkDir(*workDir))
	if err != nil {
		fmt.Fprintf(os.Stderr, "%s evidence validate: %v\n", benchCommandName(), err)
		return 1
	}
	if _, err := validator.ValidateFile(fs.Arg(0)); err != nil {
		fmt.Fprintf(os.Stderr, "%s evidence validate: %v\n", benchCommandName(), err)
		return 1
	}
	return 0
}

func cmdEvidenceAppend(args []string) int {
	fs := flagSet("evidence append")
	workDir := fs.String("work-dir", "", "Repository root (default: cwd)")
	input := fs.String("in", "", "Evidence records JSON/JSONL file")
	ledger := fs.String("ledger", "", "Append-only ledger JSONL path")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if *input == "" || *ledger == "" {
		fmt.Fprintf(os.Stderr, "Usage: %s evidence append --in <records.jsonl> --ledger <ledger.jsonl>\n", benchCommandName())
		return 2
	}

	validator, err := evidence.NewValidator(resolveWorkDir(*workDir))
	if err != nil {
		fmt.Fprintf(os.Stderr, "%s evidence append: %v\n", benchCommandName(), err)
		return 1
	}
	report, err := validator.AppendLedger(*input, *ledger)
	if report != nil && report.Added > 0 {
		fmt.Fprintf(os.Stderr, "%s evidence append: appended %d record(s) to %s\n", benchCommandName(), report.Added, report.LedgerPath)
	}
	if err != nil {
		if dupErr, ok := err.(*evidence.DuplicateRecordsError); ok {
			fmt.Fprintf(os.Stderr, "%s evidence append: %s\n", benchCommandName(), dupErr.Error())
			return 1
		}
		fmt.Fprintf(os.Stderr, "%s evidence append: %v\n", benchCommandName(), err)
		return 1
	}
	if report != nil && len(report.Duplicates) > 0 {
		fmt.Fprintf(os.Stderr, "%s evidence append: duplicate records: %s\n", benchCommandName(), strings.Join(report.Duplicates, ", "))
		return 1
	}
	return 0
}
