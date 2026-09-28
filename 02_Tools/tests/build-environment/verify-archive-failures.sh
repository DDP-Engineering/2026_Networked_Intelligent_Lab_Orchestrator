#!/usr/bin/env sh
set -eu

repo_root=$(CDPATH= cd -- "$(dirname -- "$0")/../../.." && pwd)
exec python3 -m unittest -v \
  02_Tools.tests.software.test_build_environment.BuildEnvironmentTests.test_archive_round_trip_and_corruption_fail_closed \
  02_Tools.tests.software.test_build_environment.BuildEnvironmentTests.test_pointer_missing_extra_and_reordered_parts_are_rejected
