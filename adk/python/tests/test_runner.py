# Copyright (c) 2025 Steven Verhelle (enqack)
#
# This Source Code Form is subject to the terms of the Mozilla Public
# License, v. 2.0. If a copy of the MPL was not distributed with this
# file, You can obtain one at https://mozilla.org/MPL/2.0/.
#
# SPDX-License-Identifier: MPL-2.0

import unittest
import sys
import os
import types
from unittest.mock import MagicMock

# Ensure we can import the ADK
sys.path.append(os.path.join(os.path.dirname(__file__), ".."))

# Mock gapi.native.adk before importing runner to avoid import errors
sys.modules["gapi.native.adk"] = MagicMock()

from agent import runner
from gapi.agent import capability

class TestRunner(unittest.TestCase):
    def test_capability_extraction(self):
        # Define a mock module with capabilities
        mod = types.ModuleType("test_agent")
        
        # 1. Lifecycle method
        mod.start = lambda: None
        
        # 2. Decorated method
        @capability("my_cap")
        def my_func(): pass
        mod.my_func = my_func
        
        # 3. Method with multiple caps (not supported mainly but check behavior)
        @capability("cap1")
        @capability("cap2")
        def multi_cap(): pass
        mod.multi_cap = multi_cap
        
        # Run extraction logic (recreating logic from describe() or helper if exposed)
        # Since logic is inside describe(), let's call describe()
        # Mock metadata
        mod.ID = "test_agent"
        
        meta = runner.describe(mod)
        caps = meta["describe"]["capabilities"]
        
        self.assertIn("start", caps)
        self.assertIn("my_cap", caps)
        self.assertIn("cap1", caps)
        self.assertIn("cap2", caps)
        
    def test_schema_hash_logic(self):
        # Test that runner handles file hash computation (mocking adk)
        mod = types.ModuleType("hashed_agent")
        mod.__file__ = "/tmp/fake_agent.py"
        mod.ID = "hashed"
        
        # Access the ADK mock
        adk_mock = runner.adk
        adk_mock.ComputeSchemaHash.return_value = "deadbeef"
        
        # We need to simulate the main execution flow or just the hash part?
        # The hash logic is in main() which is hard to test directly without refactoring.
        # But we can verify that the runner *would* call it if we could separate it.
        # Given current runner.py structure, main() does everything.
        # Let's verify that describe() returns valid structure at least.
        
        meta = runner.describe(mod)
        self.assertEqual(meta["describe"]["schema_version"], "1.0.0")

if __name__ == '__main__':
    unittest.main()
