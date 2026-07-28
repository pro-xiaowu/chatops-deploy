import type {ApiEnvelope,Application,Capabilities,Cluster,Environment,MessageProvider,MessageProviderSettings,Operation,User} from './types'

const token=()=>typeof localStorage==='undefined'?'':localStorage.getItem('chatops_api_token')||''
const cookie=(name:string)=>typeof document==='undefined'?'':document.cookie.split('; ').find(value=>value.startsWith(`${name}=`))?.split('=')[1]||''

async function decode<T>(response:Response):Promise<T>{
  const body=await response.json() as ApiEnvelope<T>
  if(!response.ok)throw new Error(body.error?.message||`请求失败 (${response.status})`)
  return body.data
}

async function publicRequest<T>(path:string,init?:RequestInit):Promise<T>{
  const response=await fetch(path,{credentials:'include',...init,headers:{'Content-Type':'application/json',...(init?.headers||{})}})
  return decode<T>(response)
}

async function request<T>(path:string,init?:RequestInit):Promise<T>{
  const authorization=token()
  const response=await fetch(`/api/v1${path}`,{credentials:'include',...init,headers:{'Content-Type':'application/json',...(authorization?{Authorization:`Bearer ${authorization}`}:{'X-CSRF-Token':decodeURIComponent(cookie('chatops_csrf'))}),...(init?.headers||{})}})
  return decode<T>(response)
}

export const api={
  capabilities:()=>publicRequest<Capabilities>('/auth/capabilities'),
  devLogin:()=>publicRequest<void>('/auth/dev/login',{method:'POST',body:'{}'}),
  me:()=>request<User>('/me'),
  messageProvider:()=>request<MessageProviderSettings>('/settings/message-provider'),
  selectMessageProvider:(provider:MessageProvider)=>request<{provider:MessageProvider}>('/settings/message-provider',{method:'PUT',body:JSON.stringify({provider})}),
  checkMessageProvider:(provider:MessageProvider)=>request(`/settings/message-provider/${provider}/check`,{method:'POST',body:'{}'}),
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
