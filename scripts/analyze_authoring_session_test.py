from __future__ import annotations

import importlib.util
from importlib.machinery import SourceFileLoader
import unittest
from pathlib import Path


ROOT = Path(__file__).resolve().parents[1]
SCRIPT = ROOT / "scripts" / "analyze-authoring-session"
FIXTURE = ROOT / "scripts" / "testdata" / "authoring-session-compaction.jsonl"


def load_module():
    loader = SourceFileLoader("analyze_authoring_session", str(SCRIPT))
    spec = importlib.util.spec_from_loader(loader.name, loader)
    assert spec is not None
    module = importlib.util.module_from_spec(spec)
    assert spec.loader is not None
    spec.loader.exec_module(module)
    return module


class AnalyzeAuthoringSessionTest(unittest.TestCase):
    def test_reports_compaction_causes_separately_from_normal_output(self):
        analyzer = load_module()
        commands, output_chars, compactions, large_items, token_events = analyzer.parse_lines(FIXTURE)
        summary = analyzer.analyze(commands, output_chars, compactions, large_items, token_events)

        self.assertEqual(7, summary["compactions"])
        self.assertEqual(2, summary["commands"])
        self.assertEqual(
            {
                "breyta resources read res://v1/ws/BN2/result/table/tf_820 --pretty": 2,
            },
            summary["duplicateCommands"],
        )
        self.assertEqual(summary["outputChars"], summary["normalOutputChars"])
        self.assertGreater(summary["compactionReplacementChars"], summary["normalOutputChars"])
        self.assertEqual(246100, summary["highWaterInputTokens"])
        self.assertEqual(246500, summary["highWaterTokenCount"])
        self.assertEqual(258400, summary["modelContextWindow"])
        self.assertEqual(95.2, summary["contextUsedPercent"])
        self.assertIn("automatic-influencer-research", summary["suspectedFlowSlugs"])
        self.assertGreaterEqual(len(summary["largeUserPastes"]), 1)
        self.assertGreaterEqual(len(summary["largeToolOutputs"]), 1)
        self.assertIn("report_markdown", summary["largeUserPastes"][0]["preview"])


if __name__ == "__main__":
    unittest.main()
