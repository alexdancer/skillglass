// Run against an explicitly selected installed Pi package. No extensions, agent
// session, credentials, package installs or skill workloads are executed.
import assert from 'node:assert/strict';
import fs from 'node:fs';
import os from 'node:os';
import path from 'node:path';
import { pathToFileURL } from 'node:url';
import crypto from 'node:crypto';
import { spawnRecorded } from './invocation.mjs';
const installed=process.argv[2];
if (!installed) throw new Error('usage: python3 tests/native/pi-read-contained.py INSTALLED_PI_PACKAGE EVIDENCE_DIR');
if (process.env.PI_FIXTURE_CONTAINED !== 'strict') throw new Error('native imports require the fail-closed contained launcher');
const root=fs.mkdtempSync(path.join(os.tmpdir(),'ale33-native-'));
const home=path.join(root,'home'), project=path.join(root,'repo','nested');
const agentDir=path.join(root,'separate-agent-dir');
fs.mkdirSync(project,{recursive:true});fs.mkdirSync(path.join(root,'repo','.git'),{recursive:true});
process.env.HOME=home;process.env.PI_CODING_AGENT_DIR=agentDir;
const moduleUrl=(p)=>pathToFileURL(path.join(installed,'dist',p)).href;
const {SettingsManager}=await import(moduleUrl('core/settings-manager.js'));
const {DefaultPackageManager}=await import(moduleUrl('core/package-manager.js'));
const {loadSkills,formatSkillsForPrompt}=await import(moduleUrl('core/skills.js'));
const {resolveProjectTrusted}=await import(moduleUrl('core/project-trust.js'));
const {ProjectTrustStore}=await import(moduleUrl('core/trust-manager.js'));
const write=(p,text)=>{fs.mkdirSync(path.dirname(p),{recursive:true});fs.writeFileSync(p,text)};
const skill=(base,dir,name,extra='')=>{const file=path.join(base,dir,'SKILL.md');write(file,`---\nname: ${name}\ndescription: Native disposable fixture.\n${extra}---\nNo workload.\n`);return file};
const global=path.join(agentDir,'skills'), local=path.join(project,'.pi','skills');
skill(path.join(home,'.pi','agent','skills'),'ignored','ignored-default-agent-root');
skill(global,'same-tier-a','same-tier');skill(global,'same-tier-b','same-tier');
skill(global,'collision','collision');const projectWinner=skill(local,'collision','collision');
skill(global,'inherited','inherited');skill(local,'inherited','inherited');
skill(global,'explicit','explicit','disable-model-invocation: true\n');
skill(path.join(root,'repo','.agents','skills'),'ancestor','ancestor');
skill(path.join(root,'.agents','skills'),'outside','outside');
const shared=skill(path.join(root,'shared'),'bundle','shared');
fs.symlinkSync(path.dirname(shared),path.join(local,'alias'));
fs.symlinkSync(path.dirname(shared),path.join(global,'shared-global-alias'));
const configured=path.join(root,'configured');skill(configured,'selected','selected');skill(configured,'excluded','excluded');
const pkg=path.join(root,'package');skill(path.join(pkg,'skills'),'managed','managed');write(path.join(pkg,'package.json'),JSON.stringify({name:'fixture',version:'1.2.3',pi:{skills:['skills']}}));
const globalConfig=path.join(root,'global-config');
const precedencePkg=path.join(root,'precedence-package');
write(path.join(precedencePkg,'package.json'),JSON.stringify({name:'precedence',version:'1.0.0',pi:{skills:['skills']}}));
for(const name of ['project-settings-win','project-auto-win','global-settings-win','global-auto-win']) {skill(global,name,name);skill(path.join(precedencePkg,'skills'),name,name)}
for(const name of ['project-settings-win','project-auto-win','global-settings-win']) skill(globalConfig,name,name);
for(const name of ['project-settings-win','project-auto-win']) skill(local,name,name);
skill(configured,'project-settings-win','project-settings-win');
write(path.join(agentDir,'settings.json'),JSON.stringify({defaultProjectTrust:'always',skills:[globalConfig],packages:[pkg,precedencePkg]}));
write(path.join(project,'.pi','settings.json'),JSON.stringify({skills:[configured,'!excluded','!inherited'],packages:[{source:pkg,skills:[]}]}));
const inspect=async()=>{const settings=SettingsManager.create(project,agentDir);const manager=new DefaultPackageManager({cwd:project,agentDir,settingsManager:settings});const resolved=await manager.resolve(()=>{throw new Error('install forbidden')});const loaded=loadSkills({cwd:project,agentDir,includeDefaults:false,skillPaths:resolved.skills.filter(r=>r.enabled).map(r=>r.path)});return {resolved:resolved.skills,loaded}};
const baseline=await inspect();
const rpcCommands=()=>new Promise((resolve,reject)=>{
 const child=spawnRecorded(process.execPath,[path.join(installed,'dist','bundle','cli.js'),'--mode','rpc','--offline','--no-session','--no-extensions','--no-mcp','--no-context-files','--no-tools'],{cwd:project,env:{HOME:home,PI_CODING_AGENT_DIR:path.relative(project,agentDir),PI_OFFLINE:'1',PATH:'/usr/bin:/bin',TERM:'dumb',PI_PROBE_INVOCATIONS:process.env.PI_PROBE_INVOCATIONS,PI_PROBE_PROFILE:process.env.PI_PROBE_PROFILE,PI_PROBE_PARENT_INVOCATION:process.env.PI_PROBE_PARENT_INVOCATION}});
 let output='',error='';const timer=setTimeout(()=>{child.kill('SIGTERM');reject(new Error('native RPC timeout '+error))},15000);
 child.stdout.on('data',chunk=>{output+=chunk.toString();for(const line of output.split('\n')) {try {const value=JSON.parse(line);if(value.type==='response'&&value.id==='ale33-commands') {clearTimeout(timer);child.once('close',()=>resolve(value));child.kill('SIGTERM')}} catch {}}});
 child.stderr.on('data',chunk=>{error+=chunk.toString()});child.on('error',reject);
 child.stdin.write(JSON.stringify({id:'ale33-commands',type:'get_commands'})+'\n');
});
const rpc=await rpcCommands();
assert.equal(rpc.success,true);
assert(rpc.data.commands.some(command=>command.name==='skill:collision'));
assert(!rpc.data.commands.some(command=>command.name==='skill:excluded'));
assert(!rpc.data.commands.some(command=>command.name==='skill:managed'));

assert.equal(baseline.loaded.skills.find(s=>s.name==='collision').filePath,projectWinner);
for(const [name,base] of [['project-settings-win',configured],['project-auto-win',local],['global-settings-win',globalConfig],['global-auto-win',global]]) assert.equal(baseline.loaded.skills.find(s=>s.name===name).filePath,path.join(base,name,'SKILL.md'));
assert(!baseline.loaded.skills.some(s=>s.name==='ignored-default-agent-root'));
assert.equal(baseline.loaded.skills.find(s=>s.name==='same-tier').filePath,path.join(global,'same-tier-a','SKILL.md'));
assert.equal(baseline.loaded.skills.find(s=>s.name==='inherited').filePath,path.join(global,'inherited','SKILL.md'));
assert(baseline.loaded.skills.some(s=>s.name==='selected'));
assert(!baseline.loaded.skills.some(s=>s.name==='excluded'));
assert(!baseline.loaded.skills.some(s=>s.name==='managed'));
assert(baseline.loaded.skills.some(s=>s.name==='ancestor'));
assert(!baseline.loaded.skills.some(s=>s.name==='outside'));
assert.equal(baseline.loaded.skills.filter(s=>s.name==='shared').length,1);
assert(!formatSkillsForPrompt(baseline.loaded.skills).includes('<name>explicit</name>'));
// +exact and -exact operate on reference paths, rather than canonical targets.
write(path.join(project,'.pi','settings.json'),JSON.stringify({skills:[configured,'!selected','+selected','-excluded']}));
assert(!((await inspect()).loaded.skills.some(s=>s.name==='selected')),'+selected is not an exact relative path');
write(path.join(project,'.pi','settings.json'),JSON.stringify({skills:[configured,'!selected','+'+path.join(configured,'selected')]}));
assert((await inspect()).loaded.skills.some(s=>s.name==='selected'));
write(path.join(project,'.pi','settings.json'),JSON.stringify({skills:[configured,'!selected','+'+path.join(configured,'selected'),'-'+path.join(configured,'selected')]}));
assert(!((await inspect()).loaded.skills.some(s=>s.name==='selected')));
// Project resources are absent for untrusted settings managers.
const untrusted=SettingsManager.create(project,agentDir,{projectTrusted:false});
const denied=await new DefaultPackageManager({cwd:project,agentDir,settingsManager:untrusted}).resolve(()=>{throw new Error('install forbidden')});
assert(!denied.skills.some(r=>r.metadata.scope==='project'));
assert(loadSkills({cwd:project,agentDir,includeDefaults:false,skillPaths:denied.skills.filter(r=>r.enabled).map(r=>r.path)}).skills.some(s=>s.name==='shared'));
const trustStore=new ProjectTrustStore(agentDir);
trustStore.set(path.join(root,'repo'),true);trustStore.set(project,false);
assert.equal(trustStore.get(project),false);
trustStore.set(project,null);assert.equal(trustStore.get(project),true);
const noDecision=await resolveProjectTrusted({cwd:path.join(root,'repo'),trustStore,defaultProjectTrust:'ask',projectTrustContext:{hasUI:false}});
assert.equal(noDecision,true);
const empty=fs.mkdtempSync(path.join(os.tmpdir(),'ale33-empty-'));
assert.equal(await resolveProjectTrusted({cwd:empty,trustStore,defaultProjectTrust:'never',projectTrustContext:{hasUI:false}}),true);
fs.rmSync(empty,{recursive:true});
// Identical lexical resources retain the package's first insertion, even when
// higher-priority configured paths would otherwise win canonical selection.
write(path.join(project,'.pi','settings.json'),JSON.stringify({}));
write(path.join(agentDir,'settings.json'),JSON.stringify({defaultProjectTrust:'always',skills:[path.join(pkg,'skills')],packages:[{source:pkg,skills:[]}]}));
const overlap=await inspect();
const managedResource=overlap.resolved.find(r=>r.path===path.join(pkg,'skills','managed','SKILL.md'));
assert(managedResource && !managedResource.enabled && managedResource.metadata.origin==='package');
assert(!overlap.loaded.skills.some(s=>s.name==='managed'));
const overlapRpc=await rpcCommands();
assert(overlapRpc.success && !overlapRpc.data.commands.some(c=>c.name==='skill:managed'));
// Fallback names belong to the lexical occurrence, separately from identity.
const target=path.join(root,'canonical-target');
write(path.join(target,'SKILL.md'),'---\ndescription: Inert alias fixture.\n---\nNo workload.\n');
fs.symlinkSync(target,path.join(global,'native-alias'));
skill(global,'independent','canonical-target');
write(path.join(agentDir,'settings.json'),JSON.stringify({defaultProjectTrust:'always'}));
const alias=await inspect();
assert(alias.loaded.skills.some(s=>s.name==='native-alias' && s.filePath===path.join(global,'native-alias','SKILL.md')));
assert(alias.loaded.skills.some(s=>s.name==='canonical-target' && s.filePath===path.join(global,'independent','SKILL.md')));
const aliasRpc=await rpcCommands();
assert(aliasRpc.success && aliasRpc.data.commands.some(c=>c.name==='skill:native-alias') && aliasRpc.data.commands.some(c=>c.name==='skill:canonical-target'));
// A trusted excluded alias retains the canonical resource before an enabled
// global alias; denied trust instead permits the global occurrence.
write(path.join(project,'.pi','settings.json'),JSON.stringify({skills:['-'+path.join(local,'alias')]}));
const excludedAlias=await inspect();
assert(!excludedAlias.loaded.skills.some(s=>s.name==='shared'));

const checks=['empty-project-needs-no-trust','five-tier-name-precedence','independent-agent-dir-no-default-fallback','same-tier-first-name-filesystem-order','project-exclusion-does-not-disable-global','configured-roots-and-exclusions','package-project-filter-overrides-global','git-boundary-and-ancestor-skills','symlink-deduplication','explicit-only-prompt','exact-inclusion-exclusion-order','untrusted-project-gating','denied-project-alias-preserves-global-reference','nearest-trust-decision','native-cli-rpc-commands-offline-no-credentials','package-configured-identical-lexical-exclusion','alias-fallback-and-independent-name-rpc','trusted-excluded-alias-canonical-dedup'];
const version=JSON.parse(fs.readFileSync(path.join(installed,'package.json'),'utf8')).version;
const hashes={};for (const f of ['core/package-manager.js','core/skills.js','core/settings-manager.js','core/trust-manager.js','core/project-trust.js']) hashes[f]=crypto.createHash('sha256').update(fs.readFileSync(path.join(installed,'dist',f))).digest('hex');
console.log(JSON.stringify({version,installed,root,checks,hashes,rpc,baseline,overlap,overlapRpc,alias,aliasRpc,excludedAlias},null,2));
fs.rmSync(root,{recursive:true});
