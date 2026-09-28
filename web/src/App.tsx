import { useEffect, useMemo, useState } from 'react'
import Workbench from './Workbench'
import { AlertCircle, ArrowRight, BarChart3, Bell, BookOpen, CalendarDays, Check, ChevronDown, ChevronRight, CircleHelp, ClipboardCheck, Clock3, FileText, GraduationCap, LayoutDashboard, LifeBuoy, LogOut, Menu, MessageSquareText, MoreHorizontal, Presentation, Search, Send, Settings, ShieldCheck, Upload, UserRound, Users, X } from 'lucide-react'

type Role = { Name: string; ScopeType: string; ScopeID: string }
type Me = { ID: string; Email: string; Name: string; MustChange: boolean; Roles: Role[] }
type Action = { id:string; title:string; type:string; due:string; health:string; milestoneId:string; milestone:string }
type DashboardData = { enrolment: { EnrolmentID:string; Programme:string; Cohort:string; State:string; PlannedCompletion:string; Completed:number; Total:number; Unread:number }; actions:Action[]; staff?:boolean; counts?:Record<string,number> }
type Milestone = { id:string; title:string; description:string; stage:string; start:string; due:string; state:string; revised:boolean }
type SupportCase = { id:string; category:string; summary:string; restricted:boolean; status:string; followUp:string; owner:string }
type View = 'dashboard'|'journey'|'topics'|'supervisors'|'support'|'programme'|'reviews'|'meetings'|'examination'|'reports'|'administration'

const api = async <T,>(url:string, init?:RequestInit):Promise<T> => {
  const r=await fetch(url,{...init,headers:{...(init?.body instanceof FormData?{}:{'Content-Type':'application/json'}),...(init?.headers||{})}})
  if(!r.ok){const p=await r.json().catch(()=>({detail:'Something went wrong.'}));throw new Error(p.detail||'Something went wrong.')}
  return r.status===204?undefined as T:r.json()
}

function App(){
  const [me,setMe]=useState<Me|null>(null),[loading,setLoading]=useState(true),[notice,setNotice]=useState('')
  useEffect(()=>{api<Me>('/api/v1/me').then(setMe).catch(()=>{}).finally(()=>setLoading(false))},[])
  if(loading)return <div className="app-loading"><span className="spinner"/><p>Opening your workspace…</p></div>
  if(!me)return <Login onLogin={setMe}/>
  return <Workbench me={me} onLogout={()=>api('/api/v1/auth/logout',{method:'POST'}).finally(()=>setMe(null))}/>
}

function Login({onLogin}:{onLogin:(u:Me)=>void}){
 const [email,setEmail]=useState('student@demo.pac.test'),[password,setPassword]=useState('Demo123!Change'),[error,setError]=useState(''),[busy,setBusy]=useState(false)
 async function submit(e:React.FormEvent){e.preventDefault();setBusy(true);setError('');try{onLogin(await api('/api/v1/auth/login',{method:'POST',body:JSON.stringify({email,password})}))}catch(e){setError((e as Error).message)}finally{setBusy(false)}}
 return <main className="login-page">
   <section className="login-brand" aria-label="PAC University demo">
     <div className="wordmark light"><img className="pac-logo" src="/pac-university-logo.png" alt="Pan Africa Christian University — Where Leaders are Made"/></div>
     <div className="brand-copy"><p className="eyebrow">Postgraduate progress</p><h1>A clear path through your research journey.</h1><p>See what needs your attention, collaborate with supervisors, and keep every milestone in view.</p></div>
     <div className="brand-foot"><ShieldCheck/><span>Academic records protected by role-based access</span></div>
   </section>
   <section className="login-panel">{<form className="login-card" onSubmit={submit}>
     <div className="mobile-mark wordmark"><img className="pac-logo" src="/pac-university-logo.png" alt="Pan Africa Christian University — Where Leaders are Made"/></div>
     <p className="eyebrow green">Welcome back</p><h2>Sign in to continue</h2><p className="muted">Use your university-issued account.</p>
     {error&&<div className="form-error" role="alert"><AlertCircle/> {error}</div>}
     <label>Email address<input autoFocus type="email" value={email} onChange={e=>setEmail(e.target.value)} required/></label>
     <label>Password<input type="password" value={password} onChange={e=>setPassword(e.target.value)} required/></label>
     <button className="primary wide" disabled={busy}>{busy?<><span className="spinner small"/>Signing in…</>:'Sign in'}</button>
     <button type="button" className="text-button" onClick={()=>setError('Contact the system administrator for a temporary credential reset. Existing sessions will be revoked.')}>Forgot your password?</button>
     <p className="provisioned-note">Accounts are issued by university IT. Your programme and research record are already linked. Need access? Contact your programme office.</p>
     <div className="demo-box"><strong>Demonstration account</strong><span>Student · student@demo.pac.test</span><span>Password · Demo123!Change</span></div>
   </form>}<p className="login-help"><CircleHelp/> Need help? Contact your programme office.</p></section>
 </main>
}

export default App
