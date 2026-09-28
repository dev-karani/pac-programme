import {test,expect,type Page} from '@playwright/test'

async function login(page:Page){await page.goto('/');await page.getByLabel('Email address').fill('student@demo.pac.test');await page.getByLabel('Password',{exact:true}).fill('Demo123!Change');await page.getByRole('button',{name:'Sign in',exact:true}).click();await expect(page.getByRole('heading',{name:'Your research journey',exact:true})).toBeVisible()}

test('overview shows connected progress and real red/yellow deadline flags',async({page})=>{
 await page.clock.setFixedTime(new Date('2026-09-28T09:00:00+03:00'))
 await page.route('**/api/v1/workspace/milestones',route=>route.fulfill({json:[
  {id:'completed',title:'Orientation',state:'approved',current_start:'2026-09-01',current_due:'2026-09-10'},
  {id:'current',title:'Research proposal',state:'in_progress',current_start:'2026-09-11',current_due:'2026-09-30'},
  {id:'later',title:'Data collection',state:'not_started',current_start:'2026-10-01',current_due:'2026-12-01'}
 ]}))
 await page.route('**/api/v1/workspace/tasks',route=>route.fulfill({json:[{id:'late-task',title:'Revise sampling plan',milestone_id:'current',due_at:'2026-09-27T12:00:00+03:00'},{id:'soon-task',title:'Submit proposal document',milestone_id:'current',due_at:'2026-09-30T12:00:00+03:00'}]}))
 await login(page)
 await expect(page.getByRole('list',{name:'Research journey progress'}).getByRole('listitem')).toHaveCount(3)
 await expect(page.locator('.student-track .done')).toHaveCount(1)
 await expect(page.locator('.student-track .current')).toHaveCount(1)
 await expect(page.locator('.student-deadline-row.late')).toContainText('1 day overdue')
 await expect(page.locator('.student-deadline-row.soon')).toContainText('Due in 2 days')
})

test('student journey restores sidebar sections and keeps full milestone actions',async({page})=>{
 await login(page);await page.getByRole('button',{name:'My journey',exact:true}).click()
 await page.getByRole('navigation',{name:'Journey milestones'}).getByRole('button',{name:'Research proposal',exact:true}).click()
 await expect(page.getByRole('heading',{name:'Research proposal',exact:true})).toBeVisible()
 await page.getByRole('tab',{name:'Resources',exact:true}).click();await expect(page.getByRole('heading',{name:'Resources & guidance'})).toBeVisible()
 await page.getByRole('tab',{name:'Resources',exact:true}).press('ArrowRight');await expect(page.getByRole('tab',{name:'Meetings',exact:true})).toHaveAttribute('aria-selected','true')
 await expect(page.getByRole('button',{name:'Manage meetings',exact:true})).toBeVisible()
 await page.getByRole('tab',{name:'Submissions',exact:true}).click();await expect(page.getByRole('heading',{name:'Submissions & feedback'})).toBeVisible()
 await page.getByRole('button',{name:'Open milestone',exact:true}).click();await expect(page.getByRole('dialog',{name:'Research proposal',exact:true})).toBeVisible();await page.keyboard.press('Escape')
 await expect(page.getByRole('dialog')).toHaveCount(0)
})

test('journey and overview fit a narrow mobile screen',async({page})=>{
 await page.setViewportSize({width:390,height:844});await login(page)
 expect(await page.evaluate(()=>document.documentElement.scrollWidth<=window.innerWidth)).toBeTruthy()
 await page.getByRole('button',{name:'Continue my journey'}).click();await expect(page.getByLabel('Choose a milestone')).toBeVisible()
 await page.getByLabel('Choose a milestone').selectOption({label:'3. Research proposal'})
 await page.getByRole('tab',{name:'Resources',exact:true}).click();await expect(page.getByRole('heading',{name:'Resources & guidance'})).toBeVisible()
 expect(await page.evaluate(()=>document.documentElement.scrollWidth<=window.innerWidth)).toBeTruthy()
})

test('status pills keep compact width and clear spacing in cards',async({page})=>{
 await login(page);await page.getByRole('button',{name:'Requests & support',exact:true}).click()
 const card=page.locator('.record-card').first();await expect(card).toBeVisible()
 const box=await card.evaluate(el=>{const badge=el.querySelector('.status-pill')!.getBoundingClientRect(),heading=el.querySelector('h2')!.getBoundingClientRect(),rect=el.getBoundingClientRect();return{badgeWidth:badge.width,cardWidth:rect.width,gap:heading.top-badge.bottom}})
 expect(box.badgeWidth).toBeLessThan(box.cardWidth*0.8);expect(box.gap).toBeGreaterThanOrEqual(10)
})
