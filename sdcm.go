// Code written 2024 by Hauke Bartsch.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	_ "embed"
	"errors"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"time"

	"sync/atomic"

	"github.com/iafan/cwalk"

	"github.com/suyashkumar/dicom"
	"github.com/suyashkumar/dicom/pkg/tag"
)

const version string = "0.0.1"

// The string below will be replaced during build time using
// -ldflags "-X main.compileDate=`date -u +.%Y%m%d.%H%M%S"`"
var compileDate string = ".unknown"

var own_name string = "sdcm"

var counter int32
var bytesWritten int64
var ProcessDataPath string
var InputDataPath string

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

var nonAlphanumericRegex = regexp.MustCompile(`[^a-zA-Z0-9]+`)

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

// the path we get does not have the input path prefixed
func walkFunc(path string, info os.FileInfo, err error) error {
	// func(path string, info os.FileInfo, err error) error {
	if info.IsDir() {
		return nil
	}
	if err != nil {
		return err
	}
	//fmt.Printf("\033[2J\n")

	dest_path := ProcessDataPath
	//fmt.Println("look at file: ", path)

	// Create the output path in some standard way
	oOrderPath := filepath.Join(dest_path, "input")
	if _, err := os.Stat(oOrderPath); os.IsNotExist(err) {
		err := os.Mkdir(oOrderPath, 0755)
		if err != nil {
			exitGracefully(fmt.Errorf("could not create output data directory %s", oOrderPath))
		}
	}
	in_file := filepath.Join(InputDataPath, path)

	dataset, err := dicom.ParseFile(in_file, nil, dicom.SkipPixelData()) // See also: dicom.Parse which has a generic io.Reader API.
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
				var SOPInstanceUID string
				SOPInstanceUIDVal, err := dataset.FindElementByTag(tag.SOPInstanceUID)
				if err == nil {
					SOPInstanceUID = dicom.MustGetStrings(SOPInstanceUIDVal.Value)[0]
				}

				oOrderPatientPath := filepath.Join(oOrderPath, PatientID+"_"+PatientName)
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

				outputPath := oOrderPatientDateSeriesNumber

				//inputFile, _ := os.Open(path)
				//data, _ := io.ReadAll(inputFile)
				// what is the next unused filename? We can have this case if other series are exported as well
				//fname := fmt.Sprintf("%06d.dcm", counter)
				fname := fmt.Sprintf("%s_%s.dcm", Modality, SOPInstanceUID)
				outputPathFileName := fmt.Sprintf("%s/%s", outputPath, fname)
				_, err = os.Stat(outputPathFileName)
				var c int = 0
				atomic.AddInt32(&counter, 1)
				for !os.IsNotExist(err) {
					c = c + 1 // make filename unique
					fname := fmt.Sprintf("%s_%s_%03d.dcm", Modality, SOPInstanceUID, c)
					outputPathFileName := fmt.Sprintf("%s/%s", outputPath, fname)
					//outputPathFileName := fmt.Sprintf("%s/%s_%03d.dcm", outputPath, SOPInstanceUID, c)
					_, err = os.Stat(outputPathFileName)
				}
				bw, err := copyFileContents(in_file, outputPathFileName)
				if err != nil {
					fmt.Println(err)
				}
				atomic.AddInt64(&bytesWritten, bw)
				//os.WriteFile(outputPathFileName, data, 0644)

				// We can do a better destination path here. The friendly way of doing this is
				// to provide separate folders aka the BIDS way.
				// We can create a shadow structure that uses symlinks and sorts everything into
				// sub-folders. Lets create a data view and place the info in that directory.
				symOrder := true
				if symOrder {
					symOrderPath := filepath.Join(dest_path, "input_view_dicom_series")
					if _, err := os.Stat(symOrderPath); os.IsNotExist(err) {
						err := os.Mkdir(symOrderPath, 0755)
						if err != nil {
							exitGracefully(fmt.Errorf("could not create symlink data directory %s", symOrderPath))
						}
					}
					symOrderPatientPath := filepath.Join(symOrderPath, PatientID+"_"+PatientName)
					symOrderPatientPath = strings.Replace(symOrderPatientPath, " ", ".", -1)
					if _, err := os.Stat(symOrderPatientPath); os.IsNotExist(err) {
						err := os.Mkdir(symOrderPatientPath, 0755)
						if err != nil {
							exitGracefully(fmt.Errorf("could not create symlink data directory %s", symOrderPatientPath))
						}
					}
					symOrderPatientDatePath := filepath.Join(symOrderPatientPath, StudyDate+"_"+StudyTime)
					symOrderPatientDatePath = strings.Replace(symOrderPatientDatePath, " ", ".", -1)
					if _, err := os.Stat(symOrderPatientDatePath); os.IsNotExist(err) {
						err := os.Mkdir(symOrderPatientDatePath, 0755)
						if err != nil {
							exitGracefully(fmt.Errorf("could not create symlink data directory %s", symOrderPatientDatePath))
						}
					}
					d_name := strings.Replace(SeriesNumber+"_"+SeriesDescription, "/", "_", -1)
					symOrderPatientDateSeriesNumber := filepath.Join(symOrderPatientDatePath, d_name)
					symOrderPatientDateSeriesNumber = strings.Replace(symOrderPatientDateSeriesNumber, " ", ".", -1)
					if _, err := os.Stat(symOrderPatientDateSeriesNumber); os.IsNotExist(err) {
						err := os.Mkdir(symOrderPatientDateSeriesNumber, 0755)
						if err != nil {
							exitGracefully(fmt.Errorf("could not create symlink data directory %s", symOrderPatientDateSeriesNumber))
						}
					}
					//if r, err := filepath.Rel(dest_path, symOrderPatientDateSeriesNumber); err == nil {
					//	description.InputViewDICOMSeriesPath = r
					//} else {
					//	description.InputViewDICOMSeriesPath = symOrderPatientDateSeriesNumber
					//}
					// now create symbolic link here to our outputPath + counter .dcm == outputPathFileName
					// this prevents any duplication of space taken up by the images
					fname := fmt.Sprintf("%s_%s.dcm", Modality, SOPInstanceUID)
					symlink := filepath.Join(symOrderPatientDateSeriesNumber, fname)
					// in some cases the symlink might already exist, we can make it unique by adding some number
					_, err = os.Stat(symlink)
					var c int = 0
					for !os.IsNotExist(err) {
						c = c + 1 // make filename unique
						fname2 := fmt.Sprintf("%s_%s_%03d.dcm", Modality, SOPInstanceUID, c)
						symlink = filepath.Join(symOrderPatientDateSeriesNumber, fname2)
						//outputPathFileName := fmt.Sprintf("%s/%s_%03d.dcm", outputPath, SOPInstanceUID, c)
						_, err = os.Stat(symlink)
					}

					// use outputPathFileName as the source of the symlink and make the symlink relative
					relativeDataPath := fmt.Sprintf("../%s", fname) // WRONG
					if r, err := filepath.Rel(symOrderPatientDateSeriesNumber, outputPath); err == nil {
						relativeDataPath = r
						relativeDataPath = filepath.Join(relativeDataPath, fname)
					}
					if err = os.Symlink(relativeDataPath, symlink); err != nil {
						fmt.Printf("Warning: could not create symlink %s for %s, %s\n", symlink, relativeDataPath, err)
					}
				}
				//fmt.Println("path: ", fmt.Sprintf("%s/%06d.dcm", outputPath, counter))
				//counter = counter + 1
			}
		}
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
	destination_path := dest_path + "/input"

	if _, err := os.Stat(destination_path); os.IsNotExist(err) {
		err := os.Mkdir(destination_path, 0755)
		if err != nil {
			exitGracefully(errors.New("could not create data directory"))
		}
	}
	// storing information in global objects
	counter = 0 // we are using this to name DICOM files, not possible here!
	bytesWritten = 0
	ProcessDataPath = dest_path
	InputDataPath = source_path
	//fmt.Printf("\033[2J\n") // clear the screen

	start := time.Now()
	err := cwalk.WalkWithSymlinks(source_path, walkFunc)
	if err != nil {
		fmt.Printf("Error : %s\n", err.Error())
	}
	fmt.Printf("done in %s [%s written]\n", time.Since(start), FormatFileSize(float64(bytesWritten), 1024.0))

	return counter
}

type SeriesInstanceUIDWithName struct {
	SeriesInstanceUID string
	StudyInstanceUID  string
	PatientName       string
	Name              string
	Order             int
}

func main() {

	//rand.Seed(time.Now().UnixNano())
	// disable logging
	//log.SetFlags(0)
	//log.SetOutput(io.Discard /*ioutil.Discard*/)

	own_name = os.Args[0]

	if len(os.Args) < 3 {
		exitGracefully(errors.New("Usage: <input path> <output path>"))
		os.Exit(-1)
	}
	input, err := filepath.Abs(os.Args[1])
	if err != nil {
		exitGracefully(errors.New("input path not found"))
	}
	// we will error out of the output path exists already
	if _, err := os.Stat(filepath.Join(os.Args[2], "input")); err == nil {
		exitGracefully(errors.New("output path already exists"))
	}

	fmt.Printf("Sort %s...\n", input)
	numFiles := sort(input, os.Args[2])
	fmt.Printf("%d sorted files in: [%s]\n", numFiles, ProcessDataPath)
}
