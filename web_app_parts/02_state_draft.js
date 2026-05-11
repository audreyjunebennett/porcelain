/* ── State ── */
var mobileConvos=[],continueConvos=[],grokConvos=[],cursorConvos=[],archivedConvos=[];
var searchResults=null;
var MAX_IMAGE_ATTACHMENTS=1,MAX_FILE_ATTACHMENTS=5,MAX_TOTAL_ATTACHMENTS=6;
var pendingAttachments=[]; /* { type:'image'|'file', name, imageBase64?, fileText?, fileBase64?, fileMime? } */
function pendingImage(){return pendingAttachments.find(function(a){return a.type==='image';});}
function pendingFiles(){return pendingAttachments.filter(function(a){return a.type==='file';});}
function canAddImage(){return !pendingImage()&&pendingAttachments.length<MAX_TOTAL_ATTACHMENTS;}
function canAddFile(){return pendingFiles().length<MAX_FILE_ATTACHMENTS&&pendingAttachments.length<MAX_TOTAL_ATTACHMENTS;}
var currentId=null,currentSrc='mobile',currentRO=false;
var showArchive=false;
var currentMessages=[],currentConvoTitle='';
var currentBranchIndex=0,branchCount=1;
var currentFeedback={};
var CHAT_MODE_KEY='claudia_chat_mode';
var currentMode=(function(){try{var s=localStorage.getItem(CHAT_MODE_KEY);if(s==='therapist'||s==='learning'||s==='bestie')return s;}catch(e){}return 'bestie';})();
/* ── Chat input draft (localStorage) ── */
var DRAFT_KEY='claudia_chat_draft';
function getDrafts(){ try{ var s=localStorage.getItem(DRAFT_KEY); return s?JSON.parse(s):{}; }catch(e){ return {}; } }
function draftKey(id,src){ return (src||'mobile')+'_'+(id||''); }
function getDraft(id,src){ return getDrafts()[draftKey(id,src)]||''; }
function setDraft(id,src,text){ var o=getDrafts(); if(text)o[draftKey(id,src)]=text; else delete o[draftKey(id,src)]; try{ localStorage.setItem(DRAFT_KEY,JSON.stringify(o)); }catch(e){} }
function clearDraft(id,src){ setDraft(id,src,''); }
var _draftSaveTimer=null;
function saveDraftDebounced(){ if(currentSrc!=='mobile'||!currentId||!msgInput)return; var t=msgInput.value; clearTimeout(_draftSaveTimer); _draftSaveTimer=setTimeout(function(){ setDraft(currentId,currentSrc,t); _draftSaveTimer=null; },1000); }
var _draftRestoredHintEl=null;
function showDraftRestoredHint(){ if(_draftRestoredHintEl){ _draftRestoredHintEl.style.display='block'; clearTimeout(_draftRestoredHintEl._hideAt); } else { _draftRestoredHintEl=document.createElement('div'); _draftRestoredHintEl.setAttribute('role','status'); _draftRestoredHintEl.className='draft-restored-hint'; _draftRestoredHintEl.style.cssText='padding:8px 12px;margin:0 12px 8px;background:rgba(255,122,217,.15);border:1px solid rgba(255,122,217,.35);border-radius:10px;font-size:13px;color:var(--pink,#ff7ad9);'; _draftRestoredHintEl.textContent='Restored unsent message'; var inputArea=document.getElementById('inputArea'); if(inputArea&&inputArea.parentNode)inputArea.parentNode.insertBefore(_draftRestoredHintEl,inputArea); _draftRestoredHintEl.addEventListener('click',function(){ hideDraftRestoredHint(); }); } _draftRestoredHintEl._hideAt=setTimeout(function(){ hideDraftRestoredHint(); },5000); }
function hideDraftRestoredHint(){ if(_draftRestoredHintEl){ _draftRestoredHintEl.style.display='none'; if(_draftRestoredHintEl._hideAt)clearTimeout(_draftRestoredHintEl._hideAt); } }
function restoreDraftForConvo(id,src){ if(src!=='mobile'||!id||!msgInput)return; var text=getDraft(id,src); if(!text)return; msgInput.value=text; autoResize(); showDraftRestoredHint(); updateContextIndicator(); }
