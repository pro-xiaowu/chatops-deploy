import type {ApiEnvelope,Application,Cluster,Environment,Operation,User} from './types'

const token=()=>localStorage.getItem('chatops_api_token')||''
const cookie=(name:string)=>document.cookie.split('; ').find(v=>v.startsWith(`${name}=`))?.split('=')[1]||''
async function request<T>(path:string,init?:RequestInit):Promise<T>{const response=await fetch(`/api/v1${path}`,{credentials:'include',...init,headers:{'Content-Type':'application/json',...(token()?{Authorization:`Bearer ${token()}`}:{'X-CSRF-Token':decodeURIComponent(cookie('chatops_csrf'))}) ,...(init?.headers||{})}});const body=await response.json() as ApiEnvelope<T>;if(!response.ok)throw new Error(body.error?.message||`请求失败 (${response.status})`);return body.data}
export const api={
  clusters:()=>request<Cluster[]>('/clusters'),
  createCluster:(input:unknown)=>request<Cluster>('/clusters',{method:'POST',body:JSON.stringify(input)}),
  applications:()=>request<Application[]>('/applications'),
  createApplication:(input:unknown)=>request<Application>('/applications',{method:'POST',body:JSON.stringify(input)}),
  environments:(applicationId?:string)=>request<Environment[]>(`/environments${applicationId?`?application_id=${applicationId}`:''}`),
  createEnvironment:(input:unknown)=>request<Environment>('/environments',{method:'POST',body:JSON.stringify(input)}),
  operations:()=>request<Operation[]>('/operations'),
  createOperation:(input:unknown)=>request<Operation>('/operations',{method:'POST',headers:{'Idempotency-Key':crypto.randomUUID()},body:JSON.stringify(input)}),
  approve:(id:string,input:unknown)=>request<void>(`/operations/${id}/approval`,{method:'POST',body:JSON.stringify(input)}),
  users:()=>request<User[]>('/users'),
  createUser:(input:unknown)=>request<User>('/users',{method:'POST',body:JSON.stringify(input)}),
}
