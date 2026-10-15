import fs from 'node:fs';
import path from 'node:path';
import crypto from 'node:crypto';
import { spawn } from 'node:child_process';

const digest = bytes => crypto.createHash('sha256').update(bytes).digest('hex');
const identify = file => {
 const resolved = fs.realpathSync(file);
 return {path:resolved, sha256:digest(fs.readFileSync(resolved))};
};
const persist = (directory,id,kind,value) => {
 const fd = fs.openSync(path.join(directory,`${id}.${kind}.json`),'wx',0o600);
 try {fs.writeFileSync(fd,JSON.stringify(value,null,2)+'\n');fs.fsyncSync(fd)} finally {fs.closeSync(fd)}
};
export function spawnRecorded(executable,args,options) {
 const environment = {...options.env};
 const directory = environment.PI_PROBE_INVOCATIONS;
 if (!directory || !options.cwd) throw new Error('native launch needs finalized evidence and cwd');
 const parent = JSON.parse(fs.readFileSync(environment.PI_PROBE_PARENT_INVOCATION,'utf8'));
 const profile = fs.readFileSync(environment.PI_PROBE_PROFILE);
 if (digest(profile)!==parent.profile.sha256) throw new Error('inherited profile identity mismatch');
 const id = crypto.randomUUID();
 environment.PI_PROBE_PARENT_INVOCATION = path.join(directory,`${id}.invocation.json`);
 const invocation = {launch_id:id,argv:[executable,...args],cwd:options.cwd,environment,
  executables:[identify(executable)],artifacts:[identify(args[0])],
  profile:{bytes_base64:profile.toString('base64'),sha256:digest(profile),inherited_from:parent.launch_id}};
 persist(directory,id,'invocation',invocation);
 const stdout=[],stderr=[];
 let failure;
 let child;
 try {child=spawn(executable,args,{...options,env:environment})} catch(error) {
  persist(directory,id,'completion',{launch_id:id,exit_code:-1,error:String(error),stdout_sha256:digest(Buffer.alloc(0)),stderr_sha256:digest(Buffer.alloc(0))});throw error;
 }
 child.stdout.on('data',chunk=>stdout.push(chunk));child.stderr.on('data',chunk=>stderr.push(chunk));
 child.on('error',error=>{failure=String(error)});
 child.on('close',(code,signal)=>persist(directory,id,'completion',{launch_id:id,exit_code:code,signal,error:failure,
  stdout_sha256:digest(Buffer.concat(stdout)),stderr_sha256:digest(Buffer.concat(stderr))}));
 return child;
}
