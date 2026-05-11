/* ── Tab bar (Chat / Room) ── */
var currentTab='chat';
var bedroomTick=null;
var bedroomWalkEnd=null;
var WAYPOINTS=[{id:'bed',x:52,y:72},{id:'tv',x:48,y:40},{id:'plants',x:20,y:52},{id:'window',x:76,y:48},{id:'center',x:44,y:58}];
var ACTIVITIES={bed:'\uD83D\uDCA4',tv:'\uD83D\uDCFA',plants:'\uD83C\uDF3F',window:'\u2615',center:''};
function setSpritePosition(xPct,yPct){
  if(!claudiaSprite)return;
  claudiaSprite.style.left=xPct+'%';claudiaSprite.style.top=yPct+'%';
}
function showActivity(emoji){
  if(!activityBubble)return;
  activityBubble.textContent=emoji||'';
  activityBubble.className='show'+(emoji==='\uD83D\uDCA4'?' zzz':'');
  activityBubble.setAttribute('aria-label',emoji?'Activity: '+emoji:'');
}
function hideActivity(){
  if(!activityBubble)return;
  activityBubble.className='';activityBubble.textContent='';
}
function pickNext(){
  var idx=Math.floor(Math.random()*WAYPOINTS.length);
  return WAYPOINTS[idx];
}
function runBedroomLoop(){
  if(currentTab!=='room'||!claudiaSprite||!roomPanel)return;
  var dest=pickNext();
  claudiaSprite.classList.remove('sleep');claudiaSprite.classList.add('walk');
  setSpritePosition(dest.x,dest.y);
  if(bedroomWalkEnd)clearTimeout(bedroomWalkEnd);
  bedroomWalkEnd=setTimeout(function(){
    bedroomWalkEnd=null;
    claudiaSprite.classList.remove('walk');
    var emoji=ACTIVITIES[dest.id];
    if(emoji){showActivity(emoji);if(dest.id==='bed')claudiaSprite.classList.add('sleep');}
    var duration=4000+Math.random()*4000;
    setTimeout(function(){
      hideActivity();claudiaSprite.classList.remove('sleep');
      bedroomTick=setTimeout(runBedroomLoop,800+Math.random()*1200);
    },duration);
  },2600);
}
function startBedroom(){runBedroomLoop();}
function stopBedroom(){
  if(bedroomTick){clearTimeout(bedroomTick);bedroomTick=null;}
  if(bedroomWalkEnd){clearTimeout(bedroomWalkEnd);bedroomWalkEnd=null;}
  hideActivity();
  if(claudiaSprite){claudiaSprite.classList.remove('walk','sleep');}
}
function switchToTab(tab){
  currentTab=tab;
  if(!appEl)return;
  appEl.classList.toggle('show-room',tab==='room');
  if(roomPanel)roomPanel.setAttribute('aria-hidden',tab!=='room');
  var tabs=tabBar?tabBar.querySelectorAll('.tab'):[];
  tabs.forEach(function(t){var isActive=t.getAttribute('data-tab')===tab;t.classList.toggle('active',isActive);t.setAttribute('aria-selected',isActive);});
  if(tab==='room'){startBedroom();}else{stopBedroom();}
  if(hdrName)hdrName.textContent=currentConvoTitle||'Chat';
  if(hdrSub)hdrSub.textContent=currentSrc==='mobile'?'Claudia':(currentSrc||'');
  if(currentId)loadConvo(currentId,currentSrc);
  else{clearChat();showEmpty();}
  updateCopyConvoButtonVisibility();
}
if(tabBar){
  tabBar.querySelectorAll('.tab').forEach(function(btn){
    btn.addEventListener('click',function(){var t=btn.getAttribute('data-tab');if(t)switchToTab(t);});
  });
}
