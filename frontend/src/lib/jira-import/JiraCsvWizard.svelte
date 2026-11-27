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
  import { t } from '../stores/i18n.svelte.js';
  import { jiraCsv } from '../api/jiraCsv.js';
  import { toExternal } from '../runtime/contextPath.js';
  import { suggestColumns, workspaceSuggestion } from './csvMapping.js';

  let { jobId = '', onClose } = $props();
  const csv = (key, params = {}) => t(`systemImport.csv.${key}`, params);
  const steps = $derived([t('common.upload'), csv('destination'), csv('columns'), csv('values'), t('common.preview'), t('testing.results')]);
  const fieldTypeLabel = (type) => type === 'richtext' ? csv('richText') : t(`pickers.fieldTypes.${type}`);
  const systemLabels = {type:'pickers.fields.type.name',status:'common.status',priority:'common.priority',due:'common.dueDate',points:'items.storyPoints'};
  const fieldLabel = (k) => systemLabels[k] ? t(systemLabels[k]) : custom[k]?.name;
  const delimiters = {comma:',',semicolon:';',tab:'⇥',pipe:'|'};
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
    opt('', t('jiraImport.mapping.skip')),opt('key',csv('jiraKey')),opt('title',t('common.title')),opt('description',t('common.description')),
    opt('comments',csv('internal')),
    ...['assignee','reporter'].flatMap(role => deployment==='cloud'
      ? [opt(role,`${t(`pickers.fields.${role}.name`)} · ${csv('cloudId')}`)]
      : ['name','key'].map(kind=>opt(`${role}-${kind}`,`${t(`pickers.fields.${role}.name`)} · ${csv(kind==='name'?'dcLogin':'dcKey')}`))),
    ...Object.entries(custom).map(([k,v]) => opt(k,`${k.startsWith('cf:') || k.startsWith('new:') ? `${t('pickers.custom')} · ` : ''}${fieldLabel(k)} (${fieldTypeLabel(v.type)})`)),
    opt('create',csv('newField'))
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
      key:destination?'':key.trim().toUpperCase(),headers:staged.headers,columns,values,people,new_fields:Object.fromEntries(Object.entries(newFields).filter(([k])=>columns.includes(k))),
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
    const options=custom[target]?.options||[];
    const seen=new Set(), ambiguous=new Set();
    for(const o of options){const label=o.label.toLowerCase();if(seen.has(label))ambiguous.add(label);seen.add(label);}
    return [opt(0,csv('omit')),...options.map(o=>opt(o.id,ambiguous.has(o.label.toLowerCase())?`${o.label} (#${o.id})`:o.label))];
  }
  function chosen(target,raw){
    const matches=custom[target]?.options?.filter(o=>o.label.toLowerCase()===raw.toLowerCase())||[];
    return values[target]?.[raw] ?? preview?.suggestions?.[target]?.[raw] ?? (matches.length===1?matches[0].id:'');
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
        if(!upload.total_rows)throw new Error(csv('emptyFile'));
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
  <ModalHeader title={csv('fileSource')} subtitle={file?.name || job?.filename || csv('fileHint')} onClose={onClose}/>
  <div class="px-6 py-4"><Stepper {steps} currentStep={step+1} showLabels={true} size="small"/></div>
  <div class="px-6 pb-5 overflow-auto space-y-4" style="max-height:60vh">
    {#if error}<AlertBox variant="error" message={error}/>{/if}
    {#if step===0}
      <p>{csv('exportHint')}</p>
      <FormField label={csv('fileLabel')} id="jira-csv-file"><input id="jira-csv-file" type="file" accept=".csv,.tsv" onchange={e=>file=e.currentTarget.files?.[0]}/></FormField>
      <SelectField label={csv('delimiter')} bind:value={delimiter} options={[opt('',csv('autoDetect')),...Object.entries(delimiters).map(([v,label])=>opt(v,label))]}/>
    {:else if step===1}
      <p>{csv('fileSummary',{rows:staged.total_rows,columns:staged.headers.length,delimiter:delimiters[staged.delimiter]})}</p>
      {#if staged.header_warning}<AlertBox variant="warning" message={staged.header_warning}/>{/if}
      <details><summary>{csv('sourcePreview')}</summary><div class="overflow-auto"><table><thead><tr>{#each staged.headers as h}<th>{h}</th>{/each}</tr></thead><tbody>{#each staged.preview_rows as row}<tr>{#each row as cell}<td>{cell}</td>{/each}</tr>{/each}</tbody></table></div></details>
      <SelectField label={csv('deployment')} bind:value={deployment} options={[opt('cloud','Jira Cloud'),opt('dc','Server / Data Center')]}/>
      <TextField label={csv('sourceLabel')} helper={csv('sourceHint')} dataTestid="jira-csv-source" bind:value={source}/>
      <SelectField label={csv('keyColumn')} value={keyColumn} onchange={v=>hint(v,true)} options={staged.headers.map((h,i)=>opt(i,`${i+1}. ${h}`))}/>
      <SelectField label={csv('destination')} bind:value={destination} searchable options={[opt(0,t('jiraImport.mapping.createNewWorkspace')),...(catalog.workspaces||[]).map(w=>opt(w.id,`${w.name} (${w.key})`))]}/>
      {#if !destination}
        <TextField label={t('workspaces.workspaceName')} dataTestid="jira-csv-name" bind:value={name}/>
        <TextField label={t('workspaces.workspaceKey')} dataTestid="jira-csv-key" bind:value={key} helper={keyState==='available'?t('pickers.available'):csv('keyHint')} error={keyState==='taken'?csv('keyTaken',{key}):''}/>
      {/if}
    {:else if step===2}
      <p>{csv('mapHint')}</p>
      <Button variant="ghost" size="small" onclick={()=>showEmpty=!showEmpty}>{showEmpty?csv('hideEmpty'):csv('showEmpty')}</Button>
      {#each staged.headers as h,i}
        {#if showEmpty || example(i) || columns[i]}
          <div class="grid grid-cols-2 gap-3 border-b border-ds-border py-2">
            <div><b>{i+1}. {h}</b><p class="text-sm break-words">{example(i).slice(0,160)}</p></div>
            <SelectField ariaLabel={csv('columnTarget',{index:i+1})} value={columns[i]} onchange={v=>bindColumn(i,v)} searchable options={targets} dataTestid={`jira-csv-column-${i}`}/>
          </div>
        {/if}
      {/each}
      {#each Object.entries(newFields).filter(([k])=>columns.includes(k)) as [k,f]}
        <section class="space-y-2"><h3>{t('fields.createField')}</h3>
          <TextField label={t('fields.fieldName')} value={f.name} oninput={e=>newFields[k].name=e.currentTarget.value}/>
          <SelectField label={t('fields.fieldType')} value={f.type} onchange={v=>newFields[k].type=v} options={fieldTypes.map(v=>opt(v,fieldTypeLabel(v)))}/>
          {#if ['select','multiselect'].includes(f.type)}<TextField label={csv('options')} value={f.options.map(o=>o.label).join('|')} oninput={e=>newFields[k].options=e.currentTarget.value.split('|').map((s,i)=>({id:i+1,label:s.trim()}))}/>{/if}
        </section>
      {/each}
      <SelectField label={csv('dateFormat')} bind:value={dateFormat} options={[opt('jira','14/Mar/27 12:00 AM'),opt('iso','2027-03-14'),opt('dmy','14/03/2027 15:00'),opt('mdy','03/14/2027 15:00')]}/>
      <TextField label={csv('timezone')} bind:value={timezone}/>
      {#if columns.includes('comments')}
        <AlertBox variant="info" message={csv('visibility')}/>
        {#if deployment==='dc'}<SelectField label={csv('authorId')} bind:value={authorKind} options={[opt('unknown',csv('unknown')),opt('name',csv('dcLogin')),opt('key',csv('dcKey'))]}/>{/if}
      {/if}
    {:else if step===3}
      {#each Object.entries(preview.choices||{}) as [target, choices]}
        <h3 class="font-semibold">{fieldLabel(target)}</h3>
        {#each Object.entries(choices) as [raw,count]}
          <SelectField label={`${raw} (${count})`} value={chosen(target,raw)} onchange={v=>setValue(target,raw,v)} options={fieldOptions(target)} searchable placeholder={csv('chooseValue')}/>
        {/each}
      {/each}
      {#if Object.keys(preview.people||{}).length}<h3 class="font-semibold">{t('pickers.fieldCategories.people')}</h3>{/if}
      {#each Object.entries(preview.people||{}) as [ref,id]}
        <SelectField label={preview.people_labels?.[ref]?`${preview.people_labels[ref]} · ${ref}`:ref} helper={ref.startsWith('name:') && id>0 && people[ref]===undefined && catalog.users.find(u=>u.id===id)?.username.toLowerCase()===ref.slice(5).toLowerCase()?csv('matched'):''}
          searchable value={people[ref] ?? id} onchange={v=>people={...people,[ref]:Number(v)}}
          options={[opt(0,csv('unresolved')),...(ref.startsWith('unknown:')?[]:[opt(-1,csv('inactive'))]),...(catalog.users||[]).map(u=>opt(u.id,u.username))]}/>
      {/each}
    {:else if step===4}
      <p><b>{file.name}</b> → {destination?catalog.workspaces.find(w=>w.id===destination)?.name:`${t('common.create')}: ${name} (${key})`}</p>
      <p data-testid="jira-csv-review">{csv('reviewCounts',preview)}</p>
      {#each preview.errors||[] as message}<p>{message}</p>{/each}
      {#each preview.samples||[] as row}<section class="border-t border-ds-border pt-2"><b>{row.key} · {row.title}</b><p class="whitespace-pre-wrap">{row.description}</p>
        {#each Object.entries(row.fields||{}) as [k,v]}<p>{fieldLabel(k)}: {Array.isArray(v)?v.map(id=>custom[k]?.options?.find(o=>o.id===id)?.label??id).join(', '):custom[k]?.options?.find(o=>o.id===v)?.label??v}</p>{/each}
        {#each row.comments||[] as c}<p class="whitespace-pre-wrap">{csv('internal')} · {c.author} · {c.date}: {c.body}</p>{/each}
      </section>{/each}
      <details><summary>{csv('skippedColumns',{count:columns.filter(v=>!v).length})}</summary>{staged.headers.filter((_,i)=>!columns[i]).join(', ')}</details>
    {:else if job}
      <p data-testid="jira-csv-result"><b>{job.status==='completed_with_errors'?t('jiraImport.import.completeWithErrors'):t(`systemImport.statuses.${job.status}`)}</b> · {csv('resultCounts',{created:job.imported_count,skipped:job.skipped_count,failed:job.failed_count})}</p>
      {#if job.error}<AlertBox variant="error" message={job.error}/>{/if}
      {#each job.errors||[] as message}<p>{message}</p>{/each}
      {#if job.created?.length}
        <h3>{csv('createdIssues')}{job.imported_count>job.created.length?' (50)':''}</h3>
        {#each job.created as item}<p>{item.source} → <a href={toExternal(`/item/${item.key}`)}>{item.key}</a></p>{/each}
      {/if}
      {#if job.workspace_id}<a href={toExternal(`/workspaces/${job.workspace_id}`)}>{csv('openDestination')}</a>{/if}
    {:else}<p>{t('common.loading')}</p>{/if}
  </div>
  <div class="px-6 py-4 flex gap-2 justify-end">
    <Button variant="secondary" disabled={busy} onclick={onClose}>{step===5?t('common.close'):t('common.cancel')}</Button>
    {#if step>0 && step<5}<Button variant="secondary" disabled={busy} onclick={()=>{step=step===4&&!valuesStep?2:step-1;error=''}}>{t('common.back')}</Button>{/if}
    {#if step<5}<Button dataTestid="jira-csv-next" loading={busy} disabled={locked} onclick={next}>{step===4?csv(destination?'importN':'createImportN',{count:preview.ready}):step===3?t('common.preview'):t('common.continue')}</Button>{/if}
  </div>
</Modal>
