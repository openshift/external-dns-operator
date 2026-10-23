const assert = require('node:assert/strict');
const config = require('../renovate.json');

const automaticMergeLabel = 'allow-automatic-merge';
const eligibleRules = config.tekton.packageRules.filter(rule =>
  rule.addLabels?.includes(automaticMergeLabel));
assert.equal(eligibleRules.length, 1);
const eligibleRule = eligibleRules[0];
assert.deepEqual(eligibleRule.matchManagers, ['tekton']);
assert.deepEqual(eligibleRule.matchJsonata, ["baseBranch = 'main'"]);
assert.deepEqual(eligibleRule.matchFileNames, ['.tekton/**']);
assert.deepEqual(eligibleRule.matchUpdateTypes, ['digest']);
assert.equal(eligibleRule.branchPrefix, 'konflux/tekton-digests/');
assert.equal(eligibleRule.additionalBranchPrefix, '');
assert.equal(eligibleRule.groupName, 'Tekton digest updates');
assert.equal(eligibleRule.group.branchTopic, '{{{baseBranch}}}');
assert.equal(config.tekton.packageRules.at(-1), eligibleRule);

for (const rule of [...config.packageRules, ...config.tekton.packageRules]) {
  if (rule === eligibleRule) continue;
  assert.ok(!rule.addLabels?.includes(automaticMergeLabel));
  assert.ok(!rule.labels?.includes(automaticMergeLabel));
  assert.notEqual(rule.branchPrefix, eligibleRule.branchPrefix);
  assert.notEqual(rule.groupName, eligibleRule.groupName);
}
assert.ok(!config.labels?.includes(automaticMergeLabel));
assert.ok(!config.addLabels?.includes(automaticMergeLabel));
assert.ok(!config.automerge);
assert.deepEqual(config.enabledManagers, ['dockerfile', 'tekton']);
assert.deepEqual(config.tekton.includePaths, ['.tekton/**']);
assert.equal(config.tekton.packageRules[0].groupName, 'Konflux references');
assert.equal(config.tekton.packageRules[0].branchPrefix, 'konflux/references/');
assert.equal(config.tekton.postUpgradeTasks.executionMode, 'branch');
assert.deepEqual(config.tekton.postUpgradeTasks.commands,
  ['pipeline-migration-tool migrate -f "$RENOVATE_POST_UPGRADE_COMMAND_DATA_FILE"']);
console.log('PASS: Tekton digest eligibility, isolated branch identity, and migration invariants');

async function verifyUpdateBehavior() {
  const { getConfig } = require('renovate/dist/config/defaults');
  const { mergeChildConfig } = require('renovate/dist/config/utils');
  const { flattenUpdates } = require('renovate/dist/workers/repository/updates/flatten');
  const { generateBranchConfig } = require('renovate/dist/workers/repository/updates/generate');
  const defaults = mergeChildConfig(getConfig(), {
    branchPrefix: 'konflux/mintmaker/',
    additionalBranchPrefix: '{{baseBranch}}/',
    labels: ['dependencies'],
    tekton: {
      packageRules: [{
        matchPackageNames: ['/^quay.io/konflux-ci/tekton-catalog//'],
        groupName: 'Konflux references',
        branchPrefix: 'konflux/references/',
        additionalBranchPrefix: '',
        group: { branchTopic: '{{{baseBranch}}}' },
        versioning: 'semver-coerced',
      }, {
        matchPackageNames: ['/^quay.io/konflux-ci/konflux-vanguard//'],
        matchUpdateTypes: ['replacement'],
        branchPrefix: 'konflux/references-replacement/',
        groupName: 'Konflux references replacement',
        group: {
          commitMessageTopic: '{{depName}} references',
          commitMessageExtra: '',
          commitBody: 'Replace vanguard references with equivalent tekton-catalog references',
        },
      }, {
        matchPackageNames: ['quay.io/konflux-ci/konflux-vanguard/task-rpms-signature-scan'],
        replacementName: 'quay.io/konflux-ci/tekton-catalog/task-rpms-signature-scan',
      }, {
        matchPackageNames: ['quay.io/konflux-ci/buildah-task'],
        replacementName: 'quay.io/konflux-ci/task-runner',
      }],
    },
  });
  const effectiveConfig = mergeChildConfig(defaults, config);
  effectiveConfig.semanticCommits = 'disabled';
  const baselineConfig = mergeChildConfig(defaults, {
    ...config,
    tekton: { ...config.tekton, packageRules: config.tekton.packageRules.slice(0, -1) },
  });
  baselineConfig.semanticCommits = 'disabled';
  let checked = 0;
  let enabledDockerfileUpdates = 0;
  const mixed = [];
  for (const baseBranch of ['main', 'release-1.1', 'release-1.2']) {
    for (const manager of ['tekton', 'dockerfile', 'kubernetes']) {
      for (const packageFile of ['.tekton/pipeline.yaml', 'other/pipeline.yaml',
        'Containerfile.external-dns-operator']) {
        for (const updateType of ['digest', 'patch', 'minor', 'major', 'pinDigest', 'replacement']) {
          for (const packageName of ['quay.io/konflux-ci/tekton-catalog/task-build',
            'quay.io/konflux-ci/konflux-vanguard/task-build', 'quay.io/example/task-build',
            'quay.io/konflux-ci/konflux-vanguard/task-rpms-signature-scan',
            'quay.io/konflux-ci/buildah-task',
            'registry.access.redhat.com/ubi9/ubi-micro',
            'registry.access.redhat.com/ubi9/go-toolset',
            'registry.access.redhat.com/ubi8/ubi-minimal']) {
            const fixture = {
              [manager]: [{ packageFile, deps: [{
                depName: packageName, packageName, datasource: 'docker',
                currentValue: '1.0.0', currentVersion: '1.0.0',
                updates: [{ updateType,
                  newValue: updateType === 'digest' ? '1.0.0' : '1.1.0',
                  newVersion: updateType === 'digest' ? '1.0.0' : '1.1.0',
                  newMajor: 1, newMinor: 1, newDigest: 'sha256:1234567890' }],
              }] }],
            };
            const updates = await flattenUpdates({ ...effectiveConfig, baseBranch }, fixture);
            const baseline = await flattenUpdates({ ...baselineConfig, baseBranch }, fixture);
            const eligible = baseBranch === 'main' && manager === 'tekton'
              && packageFile.startsWith('.tekton/') && updateType === 'digest';
            assert.equal(updates.length, baseline.length);
            if (eligible) assert.equal(updates.length, 1);
            if (manager === 'dockerfile') enabledDockerfileUpdates += updates.length;
            for (const update of updates) {
              assert.equal(update.addLabels.includes(automaticMergeLabel), eligible);
              assert.equal(update.branchName.startsWith(eligibleRule.branchPrefix), eligible);
              assert.ok(update.labels.includes('dependencies'));
              for (const field of ['versioning', 'schedule', 'postUpgradeTasks', 'replacementName',
                'recreateWhen', 'rebaseWhen', 'prBodyDefinitions', 'prBodyTemplate']) {
                assert.deepEqual(update[field], baseline[0][field]);
              }
              if (!eligible) assert.deepEqual(update, baseline[0]);
              if (eligible) {
                assert.equal(update.branchName, 'konflux/tekton-digests/main');
                assert.deepEqual(update.postUpgradeTasks.commands, config.tekton.postUpgradeTasks.commands);
              }
              mixed.push(update);
            }
            checked += 1;
          }
        }
      }
    }
  }
  assert.ok(enabledDockerfileUpdates > 0);
  const branches = Map.groupBy(mixed, update => update.branchName);
  for (const [branchName, upgrades] of branches) {
    const branch = generateBranchConfig(upgrades);
    if (!branch.addLabels.includes(automaticMergeLabel)) continue;
    assert.equal(branchName, 'konflux/tekton-digests/main');
    assert.ok(branch.upgrades.every(update => update.manager === 'tekton'
      && update.baseBranch === 'main' && update.packageFile.startsWith('.tekton/')
      && update.updateType === 'digest'));
  }
  console.log(`PASS: ${checked} real Renovate update cases and mixed-group label isolation`);
}

verifyUpdateBehavior().catch(error => {
  console.error(error);
  process.exitCode = 1;
});
