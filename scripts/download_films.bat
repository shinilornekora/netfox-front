@echo off
chcp 65001 >nul
title Plenka: загрузка фильмов
cd /d "%~dp0..\deploy\media"
echo Загружаю 29 файлов, около 11.9 ГБ. Источник: archive.org (общественное достояние).
echo Окно можно закрыть в любой момент: при следующем запуске загрузка продолжится с того же места.
echo Сайт подхватывает каждый фильм сразу после загрузки — перезапускать ничего не нужно.
echo.
if exist "trip-to-the-moon.mp4" (echo [1/29] trip-to-the-moon.mp4 уже есть) else (
  echo [1/29] trip-to-the-moon.mp4, 49 МБ
  curl -L --fail --retry 5 --retry-delay 5 -C - -o "trip-to-the-moon.mp4.part" "https://archive.org/download/Levoyagedanslalune/Le_voyage_dans_la_lune_A_trip_to_the_moon__Georges_Mlis_1902_512kb.mp4" && move /y "trip-to-the-moon.mp4.part" "trip-to-the-moon.mp4" >nul
)
if exist "vampires-02.mp4" (echo [2/29] vampires-02.mp4 уже есть) else (
  echo [2/29] vampires-02.mp4, 54 МБ
  curl -L --fail --retry 5 --retry-delay 5 -C - -o "vampires-02.mp4.part" "https://archive.org/download/lesVampires1915Episode2-theRingThatKills/2LesVampires-laBagueQuiTue_512kb.mp4" && move /y "vampires-02.mp4.part" "vampires-02.mp4" >nul
)
if exist "vampires-04.mp4" (echo [3/29] vampires-04.mp4 уже есть) else (
  echo [3/29] vampires-04.mp4, 124 МБ
  curl -L --fail --retry 5 --retry-delay 5 -C - -o "vampires-04.mp4.part" "https://archive.org/download/lesVampires1915Episode4-theSpectre/4LesVampires-leSpectre_512kb.mp4" && move /y "vampires-04.mp4.part" "vampires-04.mp4" >nul
)
if exist "vampires-01.mp4" (echo [4/29] vampires-01.mp4 уже есть) else (
  echo [4/29] vampires-01.mp4, 129 МБ
  curl -L --fail --retry 5 --retry-delay 5 -C - -o "vampires-01.mp4.part" "https://archive.org/download/lesVampires1915Episode1-theSeveredHead/1LesVampires_512kb.mp4" && move /y "vampires-01.mp4.part" "vampires-01.mp4" >nul
)
if exist "vampires-05.mp4" (echo [5/29] vampires-05.mp4 уже есть) else (
  echo [5/29] vampires-05.mp4, 147 МБ
  curl -L --fail --retry 5 --retry-delay 5 -C - -o "vampires-05.mp4.part" "https://archive.org/download/lesVampires1915Episode5-lvasionDuMort/5LesVampires_512kb.mp4" && move /y "vampires-05.mp4.part" "vampires-05.mp4" >nul
)
if exist "vampires-03.mp4" (echo [6/29] vampires-03.mp4 уже есть) else (
  echo [6/29] vampires-03.mp4, 162 МБ
  curl -L --fail --retry 5 --retry-delay 5 -C - -o "vampires-03.mp4.part" "https://archive.org/download/lesVampires1915Episode3-theRedCodebook/3LesVampires-leCryptogrammeRouge_512kb.mp4" && move /y "vampires-03.mp4.part" "vampires-03.mp4" >nul
)
if exist "vampires-07.mp4" (echo [7/29] vampires-07.mp4 уже есть) else (
  echo [7/29] vampires-07.mp4, 175 МБ
  curl -L --fail --retry 5 --retry-delay 5 -C - -o "vampires-07.mp4.part" "https://archive.org/download/lesVampires1915Episode7-satanus/7LesVampires-satanas_512kb.mp4" && move /y "vampires-07.mp4.part" "vampires-07.mp4" >nul
)
if exist "vampires-09.mp4" (echo [8/29] vampires-09.mp4 уже есть) else (
  echo [8/29] vampires-09.mp4, 200 МБ
  curl -L --fail --retry 5 --retry-delay 5 -C - -o "vampires-09.mp4.part" "https://archive.org/download/lesVampires1915Episode9-thePoisoner/9LesVampires-lhommeDesPoisons_512kb.mp4" && move /y "vampires-09.mp4.part" "vampires-09.mp4" >nul
)
if exist "vampires-08.mp4" (echo [9/29] vampires-08.mp4 уже есть) else (
  echo [9/29] vampires-08.mp4, 207 МБ
  curl -L --fail --retry 5 --retry-delay 5 -C - -o "vampires-08.mp4.part" "https://archive.org/download/lesVampires1915Episode8-theThunderMaster/8LesVampires-leMatreDeLaFoudre_512kb.mp4" && move /y "vampires-08.mp4.part" "vampires-08.mp4" >nul
)
if exist "caligari.mp4" (echo [10/29] caligari.mp4 уже есть) else (
  echo [10/29] caligari.mp4, 211 МБ
  curl -L --fail --retry 5 --retry-delay 5 -C - -o "caligari.mp4.part" "https://archive.org/download/DasKabinettdesDoktorCaligariTheCabinetofDrCaligari/The_Cabinet_of_Dr._Caligari_512kb.mp4" && move /y "caligari.mp4.part" "caligari.mp4" >nul
)
if exist "vampires-06.mp4" (echo [11/29] vampires-06.mp4 уже есть) else (
  echo [11/29] vampires-06.mp4, 222 МБ
  curl -L --fail --retry 5 --retry-delay 5 -C - -o "vampires-06.mp4.part" "https://archive.org/download/lesVampires1915Episode6-hypnoticEyes/6LesVampires-lesYeuxQuiFascinent_512kb.mp4" && move /y "vampires-06.mp4.part" "vampires-06.mp4" >nul
)
if exist "vampires-10.mp4" (echo [12/29] vampires-10.mp4 уже есть) else (
  echo [12/29] vampires-10.mp4, 239 МБ
  curl -L --fail --retry 5 --retry-delay 5 -C - -o "vampires-10.mp4.part" "https://archive.org/download/lesVampires1915Episode10-theTerribleWedding/10LesVampires-lesNocesSanglantes_512kb.mp4" && move /y "vampires-10.mp4.part" "vampires-10.mp4" >nul
)
if exist "man-with-camera.mp4" (echo [13/29] man-with-camera.mp4 уже есть) else (
  echo [13/29] man-with-camera.mp4, 245 МБ
  curl -L --fail --retry 5 --retry-delay 5 -C - -o "man-with-camera.mp4.part" "https://archive.org/download/ChelovekskinoapparatomManWithAMovieCamera/Chelovek_s_kino-apparatom_512kb.mp4" && move /y "man-with-camera.mp4.part" "man-with-camera.mp4" >nul
)
if exist "fantomas-01.mp4" (echo [14/29] fantomas-01.mp4 уже есть) else (
  echo [14/29] fantomas-01.mp4, 265 МБ
  curl -L --fail --retry 5 --retry-delay 5 -C - -o "fantomas-01.mp4.part" "https://archive.org/download/1913fantomas1alombredelaguillotine/1913-Fant%%C3%%B4mas_1-%%C3%%80_l%%27ombre_de_la_guillotine.mp4" && move /y "fantomas-01.mp4.part" "fantomas-01.mp4" >nul
)
if exist "general.mp4" (echo [15/29] general.mp4 уже есть) else (
  echo [15/29] general.mp4, 279 МБ
  curl -L --fail --retry 5 --retry-delay 5 -C - -o "general.mp4.part" "https://archive.org/download/TheGeneral/The_General_512kb.mp4" && move /y "general.mp4.part" "general.mp4" >nul
)
if exist "safety-last.mp4" (echo [16/29] safety-last.mp4 уже есть) else (
  echo [16/29] safety-last.mp4, 289 МБ
  curl -L --fail --retry 5 --retry-delay 5 -C - -o "safety-last.mp4.part" "https://archive.org/download/SafetyLastHaroldLloyd1923.FullMovieexcellentQuality./Safety%%20Last%%20-%%20Harold%%20Lloyd%%201923.%%20Full%%20movie%%2Cexcellent%%20quality..mp4" && move /y "safety-last.mp4.part" "safety-last.mp4" >nul
)
if exist "fantomas-02.mp4" (echo [17/29] fantomas-02.mp4 уже есть) else (
  echo [17/29] fantomas-02.mp4, 301 МБ
  curl -L --fail --retry 5 --retry-delay 5 -C - -o "fantomas-02.mp4.part" "https://archive.org/download/1913juvecontrefantomas/1913-Juve_contre_Fantomas.mp4" && move /y "fantomas-02.mp4.part" "fantomas-02.mp4" >nul
)
if exist "potemkin.mp4" (echo [18/29] potemkin.mp4 уже есть) else (
  echo [18/29] potemkin.mp4, 302 МБ
  curl -L --fail --retry 5 --retry-delay 5 -C - -o "potemkin.mp4.part" "https://archive.org/download/BattleshipPotemkin/Battleship_Potemkin_512kb.mp4" && move /y "potemkin.mp4.part" "potemkin.mp4" >nul
)
if exist "sherlock-jr.mp4" (echo [19/29] sherlock-jr.mp4 уже есть) else (
  echo [19/29] sherlock-jr.mp4, 337 МБ
  curl -L --fail --retry 5 --retry-delay 5 -C - -o "sherlock-jr.mp4.part" "https://archive.org/download/sherlockjr1924_201909/Sherlock%%20Jr.%%20-%%20Buster%%20Keaton%%20%%281924%%29%%20HD%%20%%5B720p%%5D.mp4" && move /y "sherlock-jr.mp4.part" "sherlock-jr.mp4" >nul
)
if exist "fantomas-05.mp4" (echo [20/29] fantomas-05.mp4 уже есть) else (
  echo [20/29] fantomas-05.mp4, 339 МБ
  curl -L --fail --retry 5 --retry-delay 5 -C - -o "fantomas-05.mp4.part" "https://archive.org/download/silent-le-faux-magistrat/Le%%20faux%%20magistrat.mp4" && move /y "fantomas-05.mp4.part" "fantomas-05.mp4" >nul
)
if exist "fantomas-04.mp4" (echo [21/29] fantomas-04.mp4 уже есть) else (
  echo [21/29] fantomas-04.mp4, 363 МБ
  curl -L --fail --retry 5 --retry-delay 5 -C - -o "fantomas-04.mp4.part" "https://archive.org/download/FantomasContreFantomas1914/Fant%%C3%%B4mas%%20contre%%20Fant%%C3%%B4mas%%201914.mp4" && move /y "fantomas-04.mp4.part" "fantomas-04.mp4" >nul
)
if exist "nosferatu.mp4" (echo [22/29] nosferatu.mp4 уже есть) else (
  echo [22/29] nosferatu.mp4, 377 МБ
  curl -L --fail --retry 5 --retry-delay 5 -C - -o "nosferatu.mp4.part" "https://archive.org/download/Nosferatu_most_complete_version_93_mins./Nosferatu_1922_Symphony_of_Horror_512kb.mp4" && move /y "nosferatu.mp4.part" "nosferatu.mp4" >nul
)
if exist "gold-rush.mp4" (echo [23/29] gold-rush.mp4 уже есть) else (
  echo [23/29] gold-rush.mp4, 436 МБ
  curl -L --fail --retry 5 --retry-delay 5 -C - -o "gold-rush.mp4.part" "https://archive.org/download/the-gold-rush-film-1925/The-Gold-Rush-1925.mp4" && move /y "gold-rush.mp4.part" "gold-rush.mp4" >nul
)
if exist "phantom.mp4" (echo [24/29] phantom.mp4 уже есть) else (
  echo [24/29] phantom.mp4, 442 МБ
  curl -L --fail --retry 5 --retry-delay 5 -C - -o "phantom.mp4.part" "https://archive.org/download/ThePhantomoftheOpera/Phantom_of_the_Opera_512kb.mp4" && move /y "phantom.mp4.part" "phantom.mp4" >nul
)
if exist "fantomas-03.mp4" (echo [25/29] fantomas-03.mp4 уже есть) else (
  echo [25/29] fantomas-03.mp4, 444 МБ
  curl -L --fail --retry 5 --retry-delay 5 -C - -o "fantomas-03.mp4.part" "https://archive.org/download/1913-le-mort-qui-tue/1913-Le_Mort_qui_tue.mp4" && move /y "fantomas-03.mp4.part" "fantomas-03.mp4" >nul
)
if exist "sunrise.mp4" (echo [26/29] sunrise.mp4 уже есть) else (
  echo [26/29] sunrise.mp4, 473 МБ
  curl -L --fail --retry 5 --retry-delay 5 -C - -o "sunrise.mp4.part" "https://archive.org/download/sunrise_1927/sunrise_1927.ia.mp4" && move /y "sunrise.mp4.part" "sunrise.mp4" >nul
)
if exist "joan-of-arc.mp4" (echo [27/29] joan-of-arc.mp4 уже есть) else (
  echo [27/29] joan-of-arc.mp4, 927 МБ
  curl -L --fail --retry 5 --retry-delay 5 -C - -o "joan-of-arc.mp4.part" "https://archive.org/download/the-passion-of-joan-of-arc/The%%20Passion%%20of%%20Joan%%20of%%20Arc.ia.mp4" && move /y "joan-of-arc.mp4.part" "joan-of-arc.mp4" >nul
)
if exist "metropolis.mp4" (echo [28/29] metropolis.mp4 уже есть) else (
  echo [28/29] metropolis.mp4, 1439 МБ
  curl -L --fail --retry 5 --retry-delay 5 -C - -o "metropolis.mp4.part" "https://archive.org/download/metropolis-1927-english-titles/Metropolis-1927-EnglishTitles.ia.mp4" && move /y "metropolis.mp4.part" "metropolis.mp4" >nul
)
if exist "the-kid.mp4" (echo [29/29] the-kid.mp4 уже есть) else (
  echo [29/29] the-kid.mp4, 3035 МБ
  curl -L --fail --retry 5 --retry-delay 5 -C - -o "the-kid.mp4.part" "https://archive.org/download/Brzdac1921/Brzd%%C4%%85c%%20%%281921%%29.mp4" && move /y "the-kid.mp4.part" "the-kid.mp4" >nul
)
echo.
echo Готово. Откройте любой фильм на http://localhost:8080
pause
