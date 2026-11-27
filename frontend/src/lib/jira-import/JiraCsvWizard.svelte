<script>
  import { onMount, onDestroy } from 'svelte';
  import Modal from '../dialogs/Modal.svelte';
  import ModalHeader from '../dialogs/ModalHeader.svelte';
  import Button from '../components/Button.svelte';
  import AlertBox from '../components/AlertBox.svelte';
  import Stepper from '../components/Stepper.svelte';
  import TextField from '../components/TextField.svelte';
  import SelectField from '../components/SelectField.svelte';
  import FormField from '../components/FormField.svelte';
  import { jiraCsv } from '../api/jiraCsv.js';
  import { toExternal } from '../runtime/contextPath.js';
  import { suggestColumns, workspaceSuggestion } from './csvMapping.js';

  let { jobId = '', onClose } = $props();
  const steps = ['Upload', 'Destination', 'Columns', 'Values & people', 'Review', 'Results'];
  const opt = (value, label = value) => ({value, label});
  const fieldTypes = ['text','textarea','richtext','number','date','boolean','select','multiselect'];
  let step = $state(0);
  let file = $state(null), delimiter = $state('');
  let staged = $state(null), catalog = $state({fields:{},workspaces:[],users:[]});
  let busy = $state(false), error = $state(''), deployment = $state('cloud'), source = $state('');
  let authorKind = $state('unknown'), destination = $state(0), name = $state(''), key = $state('');
  let keyColumn = $state(-1), keyState = $state(''), columns = $state([]), showEmpty = $state(false);
  let values = $state({}), people = $state({}), newFields = $state({});
  let timezone = $state(Intl.DateTimeFormat().resolvedOptions().timeZone || 'UTC'), dateFormat = $state('jira');
  let preview = $state(null), job = $state(null), timer, alive = true;
  let mappingScope='', valuesStep=false;
  const custom = $derived({...catalog.fields,...newFields});
  const targets = $derived([
    opt('', 'Skip'),opt('key','Jira Key'),opt('title','Title'),opt('description','Description'),
    opt('comments','Internal comments'),
    ...(deployment === 'cloud' ? [opt('assignee','Assignee · Cloud ID'),opt('reporter','Reporter · Cloud ID')] :
      [opt('assignee-name','Assignee · DC login'),opt('reporter-name','Reporter · DC login'),opt('assignee-key','Assignee · DC user key'),opt('reporter-key','Reporter · DC user key')]),
    ...Object.entries(custom).map(([k,v]) => opt(k,`${k.startsWith('cf:') || k.startsWith('new:') ? 'Custom · ' : ''}${v.name} (${v.type})`)),
    opt('create','Create custom field…')
  ]);
  const locked = $derived(busy || (step === 0 && !file) ||
    (step === 1 && (!source.trim() || (!destination && (!name.trim() || keyState !== 'available')))) ||
    (step === 2 && (!columns.includes('title') || !columns.includes('key'))) ||
    (step === 4 && !preview?.ready));
  onMount(() => { if(jobId) {step=5;loadJob(jobId);} });
  onDestroy(() => {alive=false;clearTimeout(timer);});
  $effect(() => {
    const candidate = key.trim();
    if(step !== 1 || destination || !/^[a-z\d]{2,10}$/i.test(candidate)){keyState='';return;}
    let current=true;
    keyState='checking';
    const timeout=setTimeout(async()=>{try{
      const available=await jiraCsv.available(candidate);
      if(current) keyState=available?'available':'taken';
    }catch(e){if(current){keyState='error';error=e.message;}}},200);
    return ()=>{current=false;clearTimeout(timeout)};
  });

  function config(){
    return {upload_id:staged.upload_id,filename:file.name,delimiter:staged.delimiter,deployment,source:source.trim(),
      author_kind:deployment==='cloud'?'cloud':authorKind,workspace_id:destination,name:destination?'':name.trim(),
      key:destination?'':key.trim().toUpperCase(),columns,values,people,new_fields:Object.fromEntries(Object.entries(newFields).filter(([k])=>columns.includes(k))),
      timezone,date_format:dateFormat};
  }
  function suggestions(){
    columns=suggestColumns(staged.headers,deployment,catalog.fields);
    if(keyColumn>=0){columns=columns.map(v=>v==='key'?'':v);columns[keyColumn]='key';}
  }
  function hint(index,replace=false){
    keyColumn=Number(index);
    const s=workspaceSuggestion(staged,keyColumn);
    if(replace || !key)key=s.key;
    if(replace || !name)name=s.name;
  }
  function bindColumn(i,target){
    if(target==='create'){
      target=`new:${i}`;
      newFields[target]={name:staged.headers[i].replace(/^Custom field \((.*)\)$/i,'$1'),type:'text',options:[]};
    }
    columns[i]=target;
  }
  function fieldOptions(target){
    return [opt(0,'Use default / omit'),...(custom[target]?.options||[]).map(o=>opt(o.id,o.label))];
  }
  function chosen(target,raw){
    return values[target]?.[raw] ?? preview?.suggestions?.[target]?.[raw] ?? custom[target]?.options?.find(o=>o.label.toLowerCase()===raw.toLowerCase())?.id ?? '';
  }
  function setValue(target,raw,id){values={...values,[target]:{...values[target],[raw]:Number(id)}};}
  function example(i){return staged.preview_rows?.find(row=>row[i])?.[i] || '';}
  async function loadJob(id){
    try{const result=await jiraCsv.job(id);if(!alive)return;job=result;error='';
      if(['queued','running'].includes(result.status))timer=setTimeout(()=>loadJob(id),1200);
    }catch(e){if(alive)error=e.message}
  }
  async function next(){
    if(locked)return;
    busy=true;error='';
    try{
      if(step===0){
        const [upload,meta]=await Promise.all([jiraCsv.upload(file,delimiter),jiraCsv.catalog()]);
        if(!upload.total_rows)throw new Error('The file contains no issue rows.');
        staged=upload;catalog=meta;
        keyColumn=staged.headers.findIndex(h=>h.trim().toLowerCase()==='issue key');
        hint(keyColumn,true);values={};people={};newFields={};destination=0;
        step=1;
      }else if(step===1){
        catalog=await jiraCsv.catalog(destination);
        const scope=JSON.stringify([staged.upload_id,destination,deployment,source.trim(),keyColumn]);
        if(scope!==mappingScope){suggestions();values={};people={};newFields={};mappingScope=scope;}
        step=2;
      }else if(step===2 || step===3){
        preview=await jiraCsv.preview(config());
        if(step===2)valuesStep=!!(Object.keys(preview.choices||{}).length || Object.keys(preview.people||{}).length);
        if(step===2 && valuesStep)step=3;
        else step=4;
      }else if(step===4){
        const id=await jiraCsv.start(config());jobId=id;step=5;await loadJob(id);
      }
    }catch(e){error=e.message;if(step===4 && e.status===409 && !destination){step=1;keyState='taken';}}
    finally{busy=false}
  }
</script>

<Modal isOpen={true} onclose={onClose} preventClose={busy} maxWidth="max-w-5xl">
  <ModalHeader title="Import Jira CSV" subtitle={file?.name || job?.filename || 'Import issues without connecting to Jira'} onClose={onClose}/>
  <div class="px-6 py-4"><Stepper {steps} currentStep={step+1} showLabels={true} size="small"/></div>
  <div class="px-6 pb-5 overflow-auto space-y-4" style="max-height:60vh">
    {#if error}<AlertBox variant="error" message={error}/>{/if}
    {#if step===0}
      <p>Filter Jira issue search to one project, then export CSV with Issue key and Summary. All fields includes descriptions and comments when present.</p>
      <FormField label="CSV file (UTF-8, up to 50 MiB)" id="jira-csv-file"><input id="jira-csv-file" type="file" accept=".csv,.tsv" onchange={e=>file=e.currentTarget.files?.[0]}/></FormField>
      <SelectField label="Delimiter" bind:value={delimiter} options={[opt('','Detect automatically'),...['comma','semicolon','tab','pipe'].map(v=>opt(v))]}/>
    {:else if step===1}
      <p>{staged.total_rows} issues · {staged.headers.length} columns · {staged.delimiter} delimiter</p>
      {#if staged.header_warning}<AlertBox variant="warning" message={staged.header_warning}/>{/if}
      <details><summary>Preview source records</summary><div class="overflow-auto"><table><thead><tr>{#each staged.headers as h}<th>{h}</th>{/each}</tr></thead><tbody>{#each staged.preview_rows as row}<tr>{#each row as cell}<td>{cell}</td>{/each}</tr>{/each}</tbody></table></div></details>
      <SelectField label="Jira deployment" bind:value={deployment} options={[opt('cloud','Jira Cloud'),opt('dc','Server / Data Center')]}/>
      <TextField label="Source instance label" helper="Use the same label on future imports from this Jira instance to reuse mappings." dataTestid="jira-csv-source" bind:value={source}/>
      <SelectField label="Issue key column" value={keyColumn} onchange={v=>hint(v,true)} options={staged.headers.map((h,i)=>opt(i,`${i+1}. ${h}`))}/>
      <SelectField label="Destination" bind:value={destination} searchable options={[opt(0,'Create a workspace'),...(catalog.workspaces||[]).map(w=>opt(w.id,`${w.name} (${w.key})`))]}/>
      {#if !destination}
        <TextField label="Workspace name" dataTestid="jira-csv-name" bind:value={name}/>
        <TextField label="Workspace key" dataTestid="jira-csv-key" bind:value={key} helper={keyState==='available'?'Available':'2–10 letters or digits'} error={keyState==='taken'?`Workspace key ${key} is already in use. Choose another key.`:''}/>
      {/if}
    {:else if step===2}
      <p>Choose a destination for each column. Repeated columns can share a multiselect field or the internal comments target.</p>
      <Button variant="ghost" size="small" onclick={()=>showEmpty=!showEmpty}>{showEmpty?'Hide':'Show'} columns empty in sample</Button>
      {#each staged.headers as h,i}
        {#if showEmpty || example(i) || columns[i]}
          <div class="grid grid-cols-2 gap-3 border-b border-ds-border py-2">
            <div><b>{i+1}. {h}</b><p class="text-sm break-words">{example(i).slice(0,160)}</p></div>
            <SelectField ariaLabel={`Destination for column ${i+1}`} value={columns[i]} onchange={v=>bindColumn(i,v)} searchable options={targets} dataTestid={`jira-csv-column-${i}`}/>
          </div>
        {/if}
      {/each}
      {#each Object.entries(newFields).filter(([k])=>columns.includes(k)) as [k,f]}
        <section class="space-y-2"><h3>Create custom field</h3>
          <TextField label="Field name" value={f.name} oninput={e=>newFields[k].name=e.currentTarget.value}/>
          <SelectField label="Field type" value={f.type} onchange={v=>newFields[k].type=v} options={fieldTypes.map(t=>opt(t))}/>
          {#if ['select','multiselect'].includes(f.type)}<TextField label="Option labels (one per |)" value={f.options.map(o=>o.label).join('|')} oninput={e=>newFields[k].options=e.currentTarget.value.split('|').map((s,i)=>({id:i+1,label:s.trim()}))}/>{/if}
        </section>
      {/each}
      <SelectField label="Jira date format" bind:value={dateFormat} options={[opt('jira','14/Mar/27 12:00 AM'),opt('iso','2027-03-14'),opt('dmy','14/03/2027 15:00'),opt('mdy','03/14/2027 15:00')]}/>
      <TextField label="Source timezone" bind:value={timezone}/>
      {#if columns.includes('comments')}
        <AlertBox variant="info" message="Comments are imported as internal. Jira CSV does not preserve their original audience."/>
        {#if deployment==='dc'}<SelectField label="Comment author identifier" bind:value={authorKind} options={[opt('unknown','Unknown — map manually'),opt('name','DC login name'),opt('key','DC user key')]}/>{/if}
      {/if}
    {:else if step===3}
      {#each Object.entries(preview.choices||{}) as [target, choices]}
        <h3 class="font-semibold">{custom[target].name}</h3>
        {#each Object.entries(choices) as [raw,count]}
          <SelectField label={`${raw} (${count})`} value={chosen(target,raw)} onchange={v=>setValue(target,raw,v)} options={fieldOptions(target)} searchable placeholder="Select a value or default"/>
        {/each}
      {/each}
      {#if Object.keys(preview.people||{}).length}<h3 class="font-semibold">People</h3>{/if}
      {#each Object.entries(preview.people||{}) as [ref,id]}
        <SelectField label={preview.people_labels?.[ref]?`${preview.people_labels[ref]} · ${ref}`:ref} helper={ref.startsWith('name:') && id>0 && people[ref]===undefined && catalog.users.find(u=>u.id===id)?.username.toLowerCase()===ref.slice(5).toLowerCase()?'Matched by username':''}
          searchable value={people[ref] ?? id} onchange={v=>people={...people,[ref]:Number(v)}}
          options={[opt(0,'Keep unresolved'),...(ref.startsWith('unknown:')?[]:[opt(-1,'Create inactive imported user')]),...(catalog.users||[]).map(u=>opt(u.id,u.username))]}/>
      {/each}
    {:else if step===4}
      <p><b>{file.name}</b> → {destination?catalog.workspaces.find(w=>w.id===destination)?.name:`Create ${name} (${key})`}</p>
      <p data-testid="jira-csv-review">{preview.ready} ready · {preview.skipped} skipped · {preview.invalid} invalid · {preview.comments} internal comments</p>
      {#each preview.errors||[] as message}<p>{message}</p>{/each}
      {#each preview.samples||[] as row}<section class="border-t border-ds-border pt-2"><b>{row.key} · {row.title}</b><p class="whitespace-pre-wrap">{row.description}</p>
        {#each Object.entries(row.fields||{}) as [k,v]}<p>{custom[k]?.name}: {Array.isArray(v)?v.map(id=>custom[k]?.options?.find(o=>o.id===id)?.label??id).join(', '):custom[k]?.options?.find(o=>o.id===v)?.label??v}</p>{/each}
        {#each row.comments||[] as c}<p class="whitespace-pre-wrap">Internal · {c.author} · {c.date}: {c.body}</p>{/each}
      </section>{/each}
      <details><summary>Skipped columns ({columns.filter(v=>!v).length})</summary>{staged.headers.filter((_,i)=>!columns[i]).join(', ')}</details>
    {:else if job}
      <p data-testid="jira-csv-result"><b>{job.status.replaceAll('_',' ')}</b> · {job.imported_count} created · {job.skipped_count} skipped · {job.failed_count} failed</p>
      {#if job.error}<AlertBox variant="error" message={job.error}/>{/if}
      {#each job.errors||[] as message}<p>{message}</p>{/each}
      {#if job.created?.length}
        <h3>Created issues{job.imported_count>job.created.length?' (first 50)':''}</h3>
        {#each job.created as item}<p>{item.source} → <a href={toExternal(`/item/${item.key}`)}>{item.key}</a></p>{/each}
      {/if}
      {#if job.workspace_id}<a href={toExternal(`/workspaces/${job.workspace_id}`)}>Open destination workspace</a>{/if}
    {:else}<p>Loading import results…</p>{/if}
  </div>
  <div class="px-6 py-4 flex gap-2 justify-end">
    <Button variant="secondary" disabled={busy} onclick={onClose}>{step===5?'Close':'Cancel'}</Button>
    {#if step>0 && step<5}<Button variant="secondary" disabled={busy} onclick={()=>{step=step===4&&!valuesStep?2:step-1;error=''}}>Back</Button>{/if}
    {#if step<5}<Button dataTestid="jira-csv-next" loading={busy} disabled={locked} onclick={next}>{step===4?`${destination?'Import':'Create workspace and import'} ${preview.ready} issues`:step===3?'Review':'Continue'}</Button>{/if}
  </div>
</Modal>
