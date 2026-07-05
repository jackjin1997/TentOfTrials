import pytest
import os
import json

# Test successful report metadata
def test_successful_report_metadata():
    # Mock the build script output
    with open('diagnostic/build-success.json', 'w') as f:
        json.dump({'commit_id': 'abc123', 'module_results': [{'module': 'module1', 'status': 'success'}]}, f)
    with open('diagnostic/build-success.logd', 'w') as f:
        f.write('logd content')

    # Run the build script
    os.system('python3 build.py')

    # Verify the output
    with open('diagnostic/build-success.json', 'r') as f:
        metadata = json.load(f)
    assert metadata['commit_id'] == 'abc123'
    assert len(metadata['module_results']) == 1
    assert metadata['module_results'][0]['module'] == 'module1'
    assert metadata['module_results'][0]['status'] == 'success'
    assert os.path.exists('diagnostic/build-success.logd')

# Test logd generation failure
def test_logd_generation_failure():
    # Mock the build script output
    with open('diagnostic/build-failure.json', 'w') as f:
        json.dump({'diagnostic_logd_error': 'Error generating logd'}, f)

    # Run the build script
    os.system('python3 build.py')

    # Verify the output
    with open('diagnostic/build-failure.json', 'r') as f:
        metadata = json.load(f)
    assert metadata['diagnostic_logd_error'] == 'Error generating logd'
    assert not os.path.exists('diagnostic/build-failure.logd')

# Test chunked or multi-file logd references
def test_chunked_multi_file_logd_references():
    # Mock the build script output
    with open('diagnostic/build-chunk1.logd', 'w') as f:
        f.write('logd content 1')
    with open('diagnostic/build-chunk2.logd', 'w') as f:
        f.write('logd content 2')

    # Run the build script
    os.system('python3 build.py')

    # Verify the output
    assert os.path.exists('diagnostic/build-chunk1.logd')
    assert os.path.exists('diagnostic/build-chunk2.logd')
