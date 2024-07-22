// Code written 2024 by Hauke Bartsch.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"bytes"
	_ "embed"
	"errors"
	"fmt"
	"io"
	"log"
	"math"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"sync"
	"time"

	"flag"
	"sync/atomic"

	"github.com/iafan/cwalk"

	"github.com/suyashkumar/dicom"
	"github.com/suyashkumar/dicom/pkg/tag"

	"net/http"
	_ "net/http/pprof"
)

const version string = "0.0.2"

// The string below will be replaced during build time using
// -ldflags "-X main.compileDate=`date -u +.%Y%m%d.%H%M%S"`"
var compileDate string = ".unknown"

var own_name string = "sdcm"

var counter int32
var counterError int32
var bytesWritten int64
var ProcessDataPath string
var InputDataPath string
var startTime time.Time
var spinner_c int = 0
var spinner = []string{"⣾ ", "⣽ ", "⣻ ", "⢿ ", "⡿ ", "⣟ ", "⣯ ", "⣷ "}
var listPatients sync.Map
var listStudies sync.Map
var listSeries sync.Map

var (
	methodFlag       string
	verboseFlag      bool
	versionFlag      bool
	outputFolderFlag string
)

func UpdateCounter(counters *sync.Map, key string) {
	val, _ := counters.LoadOrStore(key, new(int64))
	ptr := val.(*int64)
	atomic.AddInt64(ptr, 1)
}

func exitGracefully(err error) {
	fmt.Fprintf(os.Stderr, "error: %v\n", err)
	os.Exit(1)
}

func check(e error) {
	if e != nil {
		exitGracefully(e)
	}
}

type Description struct {
	NameFromSelect           string
	SeriesInstanceUID        string
	SeriesDescription        string
	StudyInstanceUID         string
	NumFiles                 int32
	Modality                 string
	PatientID                string
	PatientName              string
	SequenceName             string
	StudyDate                string
	StudyTime                string
	SeriesTime               string
	SeriesNumber             string
	ReferringPhysician       string // for the research PACS this stores the event name
	InputViewDICOMSeriesPath string
}

var nonAlphanumericRegex = regexp.MustCompile(`[^a-zA-Z0-9 ]+`)

func clearString(str string) string {
	return nonAlphanumericRegex.ReplaceAllString(str, "")
}

func copyFileContents(src, dst string) (bytesWritten int64, err error) {
	in, err := os.Open(src)
	if err != nil {
		return 0, err
	}
	defer in.Close()
	out, err := os.Create(dst)
	if err != nil {
		return 0, err
	}
	defer func() {
		cerr := out.Close()
		if err == nil {
			err = cerr
		}
	}()
	if bytesWritten, err = io.Copy(out, in); err != nil {
		return 0, err
	}
	err = out.Sync()
	return bytesWritten, err
}

func printMem() {
	var ms runtime.MemStats
	runtime.ReadMemStats(&ms)
	log.Printf("System: %8d Inuse: %8d Released: %8d Objects: %6d\n", ms.HeapSys, ms.HeapInuse, ms.HeapReleased, ms.HeapObjects)
}

func populate(keyMap *map[tag.Tag]string, in_file string) {
	// put all values into the keyMap, overwrite previous values
	// reset the keyMap
	var numKeys int = len(*keyMap)

	dcm, err := os.Open(in_file)
	if err != nil {
		exitGracefully(fmt.Errorf("unable to open %s. Error: %v", in_file, err))
	}
	defer dcm.Close()

	data, err := io.ReadAll(dcm)
	if err != nil {
		exitGracefully(fmt.Errorf("unable to read file into memory for benchmark: %v", err))
	}

	r := bytes.NewReader(data)
	p, _ := dicom.NewParser(r, int64(len(data)), nil, dicom.SkipPixelData())

	var cc int = 0
	for err == nil {
		t, err := p.Next() // there is still a copy of this tag in p.Elements after this call, not needed...
		if err != nil {
			break
		}
		// t is a dicom.Element
		val, ok := (*keyMap)[t.Tag]
		if ok {
			if val == "" {
				v := t.Value.GetValue().([]string)
				if len(v) > 0 {
					(*keyMap)[t.Tag] = v[0]
				}
				cc = cc + 1 // we will use this even if the value is an empty string
				if cc == numKeys {
					break // we are done
				}
			}
		}
	}
}

func splitPath(path string) []string {
	dir, last := filepath.Split(path)
	if dir == "" {
		return []string{last}
	}
	return append(splitPath(filepath.Clean(dir)), last)
}

// the path we get does not have the input path prefixed
func walkFunc(path string, info os.FileInfo, err error) error {
	if info == nil {
		// might happen if the directory name does not exist...
		return nil
	}

	// func(path string, info os.FileInfo, err error) error {
	if info.IsDir() {
		return nil
	}
	if err != nil {
		return err
	}
	// we can filter out files that take a long time if we allow only
	//  - files without an extension, or
	//  - files with .dcm as extension
	if filepath.Ext(path) != "" && strings.ToLower(filepath.Ext(path)) != ".dcm" {
		atomic.AddInt32(&counterError, 1)
		//fmt.Printf("ignore file: %s\n", path)
		return nil // ignore this file
	}

	//fmt.Printf("\033[2J\n")

	dest_path := ProcessDataPath
	//fmt.Println("look at file: ", path)

	// Create the output path in some standard way
	oOrderPath := dest_path
	if _, err := os.Stat(oOrderPath); os.IsNotExist(err) {
		err := os.Mkdir(oOrderPath, 0755)
		if err != nil {
			exitGracefully(fmt.Errorf("could not create output data directory %s", oOrderPath))
		}
	}
	in_file := filepath.Join(InputDataPath, path)

	// Ok, we can try to be faster if we do not read the whole set, we would like
	// to also stop parsing after we have all the keys we need.
	// BenchmarkParser_NextAPI

	/*	sT := time.Now()
		keyMap := map[tag.Tag]string{
			tag.StudyInstanceUID:  "",
			tag.SeriesInstanceUID: "",
			tag.SOPInstanceUID:    "",
			tag.PatientID:         "",
			tag.PatientName:       "",
			tag.SeriesDescription: "",
			tag.StudyDate:         "",
			tag.StudyTime:         "",
			tag.SeriesNumber:      "",
			tag.Modality:          "",
		}
		populate(&keyMap, in_file)
		fmt.Printf("populate time: %v %s\n", time.Since(sT), path) */

	//sT := time.Now()
	dataset, err := dicom.ParseFile(in_file, nil, dicom.SkipPixelData()) // See also: dicom.Parse which has a generic io.Reader API.
	//fmt.Printf("ParseFile time: %v %s\n", time.Since(sT), path)
	if err == nil {
		//printMem()
		StudyInstanceUIDVal, err := dataset.FindElementByTag(tag.StudyInstanceUID)
		if err == nil {
			var StudyInstanceUID string = dicom.MustGetStrings(StudyInstanceUIDVal.Value)[0]
			//fmt.Println("StudyInstanceUID: ", StudyInstanceUID)
			SeriesInstanceUIDVal, err := dataset.FindElementByTag(tag.SeriesInstanceUID)
			if err == nil {
				var SeriesInstanceUID string = dicom.MustGetStrings(SeriesInstanceUIDVal.Value)[0]

				var SeriesDescription string
				SeriesDescriptionVal, err := dataset.FindElementByTag(tag.SeriesDescription)
				if err == nil {
					SeriesDescription = dicom.MustGetStrings(SeriesDescriptionVal.Value)[0]
					SeriesDescription = clearString(SeriesDescription)
				}
				var PatientID string
				PatientIDVal, err := dataset.FindElementByTag(tag.PatientID)
				if err == nil {
					PatientID = dicom.MustGetStrings(PatientIDVal.Value)[0]
				}
				var PatientName string
				PatientNameVal, err := dataset.FindElementByTag(tag.PatientName)
				if err == nil {
					PatientName = dicom.MustGetStrings(PatientNameVal.Value)[0]
				}
				/*var SequenceName string
				SequenceNameVal, err := dataset.FindElementByTag(tag.SequenceName)
				if err == nil {
					SequenceName = dicom.MustGetStrings(SequenceNameVal.Value)[0]
				}*/
				var StudyDate string
				StudyDateVal, err := dataset.FindElementByTag(tag.StudyDate)
				if err == nil {
					StudyDate = dicom.MustGetStrings(StudyDateVal.Value)[0]
				}
				var StudyTime string
				StudyTimeVal, err := dataset.FindElementByTag(tag.StudyTime)
				if err == nil {
					StudyTime = dicom.MustGetStrings(StudyTimeVal.Value)[0]
				}
				/*var SeriesTime string
				SeriesTimeVal, err := dataset.FindElementByTag(tag.SeriesTime)
				if err == nil {
					SeriesTime = dicom.MustGetStrings(SeriesTimeVal.Value)[0]
				}*/
				var SeriesNumber string
				SeriesNumberVal, err := dataset.FindElementByTag(tag.SeriesNumber)
				if err == nil {
					SeriesNumber = dicom.MustGetStrings(SeriesNumberVal.Value)[0]
					if SeriesNumber == "" {
						SeriesNumber = "000"
					}
				}
				var Modality string
				ModalityVal, err := dataset.FindElementByTag(tag.Modality)
				if err == nil {
					Modality = dicom.MustGetStrings(ModalityVal.Value)[0]
					if Modality == "" {
						Modality = "UK" // unknown
					}
				}
				/*var ReferringPhysician string
				ReferringPhysicianVal, err := dataset.FindElementByTag(tag.ReferringPhysicianName)
				if err == nil {
					ReferringPhysician = dicom.MustGetStrings(ReferringPhysicianVal.Value)[0]
					if ReferringPhysician != "" {
						description.ReferringPhysician = ReferringPhysician
					}
				}*/
				// keep track of the patients, studies and series but only if we use verbose mode
				if verboseFlag {
					UpdateCounter(&listPatients, PatientID)
					UpdateCounter(&listStudies, StudyInstanceUID)
					UpdateCounter(&listSeries, SeriesInstanceUID)
				}

				var SOPInstanceUID string
				SOPInstanceUIDVal, err := dataset.FindElementByTag(tag.SOPInstanceUID)
				if err == nil {
					SOPInstanceUID = dicom.MustGetStrings(SOPInstanceUIDVal.Value)[0]
				}

				// now create the folder structure based on outputFolderFlag, treat the last entry as filename
				pps := outputFolderFlag
				pps = strings.Replace(pps, "{PatientID}", PatientID, -1)
				pps = strings.Replace(pps, "{PatientName}", PatientName, -1)
				pps = strings.Replace(pps, "{StudyDate}", StudyDate, -1)
				pps = strings.Replace(pps, "{StudyTime}", StudyTime, -1)
				pps = strings.Replace(pps, "{StudyInstanceUID}", StudyInstanceUID, -1)
				pps = strings.Replace(pps, "{SeriesInstanceUID}", SeriesInstanceUID, -1)
				pps = strings.Replace(pps, "{SeriesDescription}", SeriesDescription, -1)
				pps = strings.Replace(pps, "{StudyInstanceUID}", StudyInstanceUID, -1)
				pps = strings.Replace(pps, "{SOPInstanceUID}", SOPInstanceUID, -1)
				pps = strings.Replace(pps, "{Modality}", Modality, -1)
				pps = strings.Replace(pps, "{SeriesNumber}", SeriesNumber, -1)
				pps = strings.Replace(pps, "{counter}", fmt.Sprintf("%06d", counter), -1) // use the global counter
				pps = strings.Replace(pps, " ", "-", -1)                                  // remove spaces

				pathPieces := splitPath(pps)
				piece := 0
				oOrderPatientPath := oOrderPath
				for piece < len(pathPieces)-1 {
					oOrderPatientPath = filepath.Join(oOrderPatientPath, pathPieces[piece])
					if _, err := os.Stat(oOrderPatientPath); os.IsNotExist(err) {
						err := os.Mkdir(oOrderPatientPath, 0755)
						if err != nil {
							exitGracefully(fmt.Errorf("could not create data directory %s", oOrderPatientPath))
						}
					}
					piece = piece + 1
				}
				// filename is
				fname := pathPieces[len(pathPieces)-1]

				/*				oOrderPatientPath := filepath.Join(oOrderPath, PatientID+"_"+PatientName)
								if _, err := os.Stat(oOrderPatientPath); os.IsNotExist(err) {
									err := os.Mkdir(oOrderPatientPath, 0755)
									if err != nil {
										exitGracefully(fmt.Errorf("could not create data directory %s", oOrderPatientPath))
									}
								}
								oOrderPatientDatePath := filepath.Join(oOrderPatientPath, StudyDate+"_"+StudyTime+"_"+StudyInstanceUID)
								if _, err := os.Stat(oOrderPatientDatePath); os.IsNotExist(err) {
									err := os.Mkdir(oOrderPatientDatePath, 0755)
									if err != nil {
										exitGracefully(fmt.Errorf("could not create data directory %s", oOrderPatientDatePath))
									}
								}

								d_name := strings.Replace(SeriesNumber+"_"+SeriesDescription+"_"+SeriesInstanceUID, "/", "_", -1)
								d_name = strings.Replace(d_name, " ", "", -1)
								oOrderPatientDateSeriesNumber := filepath.Join(oOrderPatientDatePath, d_name)
								if _, err := os.Stat(oOrderPatientDateSeriesNumber); os.IsNotExist(err) {
									err := os.Mkdir(oOrderPatientDateSeriesNumber, 0755)
									if err != nil {
										exitGracefully(fmt.Errorf("could not create data directory %s", oOrderPatientDateSeriesNumber))
									}
								}

								outputPath := oOrderPatientDateSeriesNumber */
				outputPath := oOrderPatientPath

				//inputFile, _ := os.Open(path)
				//data, _ := io.ReadAll(inputFile)
				// what is the next unused filename? We can have this case if other series are exported as well
				//fname := fmt.Sprintf("%06d.dcm", counter)
				// fname := fmt.Sprintf("%s_%s.dcm", Modality, SOPInstanceUID)
				outputPathFileName := fmt.Sprintf("%s/%s", outputPath, fname)
				_, err = os.Stat(outputPathFileName)
				var c int = 0
				atomic.AddInt32(&counter, 1)
				if verboseFlag && counter%100 == 0 {
					numPatients := 0
					listPatients.Range(func(key, value interface{}) bool {
						numPatients = numPatients + 1
						return true
					})
					numStudies := 0
					listStudies.Range(func(key, value interface{}) bool {
						numStudies = numStudies + 1
						return true
					})
					numSeries := 0
					listSeries.Range(func(key, value interface{}) bool {
						numSeries = numSeries + 1
						return true
					})
					fmt.Printf("\033[A\033[2K\033[94;49m%s%d\033[37m [%.0f files / s] P%04d S%04d S%04d\033[39m\033[49m\n", spinner[(spinner_c)%len(spinner)], counter, (float64(counter))/time.Since(startTime).Seconds(), numPatients, numStudies, numSeries)
					spinner_c = spinner_c + 1
				}

				for !os.IsNotExist(err) {
					c = c + 1 // make filename unique by adding a number
					fname := fmt.Sprintf("%s_%03d%s", strings.TrimSuffix(fname, filepath.Ext(fname)), c, filepath.Ext(fname))
					outputPathFileName = fmt.Sprintf("%s/%s", outputPath, fname)
					//outputPathFileName := fmt.Sprintf("%s/%s_%03d.dcm", outputPath, SOPInstanceUID, c)
					_, err = os.Stat(outputPathFileName)
				}
				var bw int64 = 0
				err = nil
				if methodFlag == "copy" {
					bw, err = copyFileContents(in_file, outputPathFileName)
				} else if methodFlag == "link" {
					if err = os.Symlink(in_file, outputPathFileName); err != nil {
						fmt.Printf("Warning: could not create symlink %s for %s, %s\n", in_file, outputPathFileName, err)
					}
				} else {
					// instead of copy we assume we want a symbolic link
					exitGracefully(errors.New("unknown option for method, we support only \"copy\" and \"link\""))
				}
				if err != nil {
					fmt.Println(err)
				}
				atomic.AddInt64(&bytesWritten, bw)
				//os.WriteFile(outputPathFileName, data, 0644)

				//fmt.Println("path: ", fmt.Sprintf("%s/%06d.dcm", outputPath, counter))
				//counter = counter + 1
			}
		}
	} else {
		atomic.AddInt32(&counterError, 1)
	}

	return nil
}

var sizes = []string{"B", "kB", "MB", "GB", "TB", "PB", "EB"}

func FormatFileSize(s float64, base float64) string {
	unitsLimit := len(sizes)
	i := 0
	for s >= base && i < unitsLimit {
		s = s / base
		i++
	}

	f := "%.0f %s"
	if i > 1 {
		f = "%.2f %s"
	}

	return fmt.Sprintf(f, s, sizes[i])
}

func sort(source_path string, dest_path string) int32 {
	destination_path := dest_path

	if _, err := os.Stat(destination_path); os.IsNotExist(err) {
		err := os.Mkdir(destination_path, 0755)
		if err != nil {
			exitGracefully(fmt.Errorf("could not create output directory %s. Output directory should exist", destination_path))
		}
	}
	// storing information in global objects
	counter = 0 // we are using this to name DICOM files, not possible here!
	counterError = 0
	bytesWritten = 0
	ProcessDataPath = dest_path
	InputDataPath = source_path
	fmt.Printf("\n")

	startTime = time.Now()
	err := cwalk.WalkWithSymlinks(source_path, walkFunc)
	if err != nil {
		fmt.Printf("Error : %s\n", err.Error())
	}
	if verboseFlag {
		sizeStr := ""
		if methodFlag != "link" {
			sizeStr = fmt.Sprintf("[%s]", FormatFileSize(float64(bytesWritten), 1024.0))
		}
		fmt.Printf("done in %s %s\n", time.Since(startTime), sizeStr)
	}

	return counter
}

type SeriesInstanceUIDWithName struct {
	SeriesInstanceUID string
	StudyInstanceUID  string
	PatientName       string
	Name              string
	Order             int
}

func IsEmpty(name string) (bool, error) {
	f, err := os.Open(name)
	if err != nil {
		return false, err
	}
	defer f.Close()

	_, err = f.Readdirnames(1) // Or f.Readdir(1)
	if err == io.EOF {
		return true, nil
	}
	return false, err // Either not empty or error, suits both cases
}

func main() {

	// Server for pprof
	go func() {
		fmt.Println(http.ListenAndServe("localhost:6060", nil))
	}()

	//rand.Seed(time.Now().UnixNano())
	// disable logging
	log.SetFlags(0)
	log.SetOutput(io.Discard /*ioutil.Discard*/)

	flag.StringVar(&methodFlag, "method", "copy", "Create symbolic links (faster) or really copy files (copy|link)")
	flag.StringVar(&outputFolderFlag, "folder", "{PatientID}_{PatientName}/{StudyDate}_{StudyTime}_{StudyInstanceUID}/{SeriesNumber}_{SeriesDescription}_{SeriesInstanceUID}/{Modality}_{SOPInstanceUID}.dcm", "Specify the requested output folder structure using the following DICOM tags:\n\t{counter}, {PatientID}, {PatientName}, {StudyDate},\n\t{StudyTime}, {SeriesDescription}, {SeriesNumber},\n\t{Modality}, {StudyInstanceUID}, {SeriesInstanceUID}, {SOPInstanceUID}\n")
	flag.BoolVar(&verboseFlag, "verbose", false, "Print more verbose output")
	flag.BoolVar(&versionFlag, "version", false, "Print the version number")
	flag.Parse()

	if versionFlag {
		timeThen := time.Now()
		setTime := false
		if compileDate != "" {
			layout := ".20060102.150405"
			t, err := time.Parse(layout, compileDate)
			if err == nil {
				timeThen = t
				setTime = true
			}
		}

		fmt.Printf("sdcm version %s%s", version, compileDate)
		if setTime {
			fmt.Printf(" build %.0f days ago\n", math.Round(time.Since(timeThen).Hours()/24))
		} else {
			fmt.Println()
		}
		os.Exit(0)
	}

	own_name = os.Args[0]

	if len(os.Args) < 3 {
		fmt.Println("Usage: <input path> <output path>")
		os.Exit(-1)
	}
	input, err := filepath.Abs(flag.Args()[0])
	if err != nil {
		exitGracefully(errors.New("input path could not be found"))
	}
	// we will error out of the output path already exists and is not empty
	if _, err := os.Stat(flag.Args()[1]); err == nil {
		isEmpty, _ := IsEmpty(flag.Args()[1])
		if !isEmpty {
			exitGracefully(fmt.Errorf("output path %s already exists, cowardly refusing to continue. Clear its content or specify a new directory", flag.Args()[1]))
		}
	}

	if outputFolderFlag == "" {
		env_folder_path := os.Getenv("SDCM_FOLDER_PATH")
		if len(env_folder_path) > 0 {
			outputFolderFlag = env_folder_path
		}
	}

	if verboseFlag {
		fmt.Printf("Parse %s...\n", input)
	}
	numFiles := sort(input, flag.Args()[1])
	if verboseFlag {
		s := "s"
		if numFiles == 1 {
			s = ""
		}
		fmt.Printf("✓ sorted %d file%s [%d non-DICOM files ignored]\n", numFiles, s, counterError)
	}
}
