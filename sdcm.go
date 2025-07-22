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
	"reflect"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"

	"flag"
	"sync/atomic"

	"github.com/iafan/cwalk"

	"github.com/suyashkumar/dicom"
	//"github.com/haukebartsch/dicom"
	//"github.com/haukebartsch/dicom/pkg/tag"

	"github.com/suyashkumar/dicom/pkg/tag"

	"golang.org/x/text/message"

	"github.com/djherbis/times"

	_ "net/http/pprof"
)

const version string = "0.0.4"

// The string below will be replaced during build time using
// -ldflags "-X main.compileDate=`date -u +.%Y%m%d.%H%M%S"`"
var compileDate string = ".unknown"

//var own_name string = "sdcm"

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
var listStructure = make(map[string][]string, 0) // map of SeriesInstanceUIDs of [PatientID, StudyDate, StudyInstanceUID, Modality]
var dicomTags map[tag.Tag]string
var preserve map[string]bool
var old_spinner_c int = 0

var listStructuresChan = make(chan []string, 1000)

var fmt_local *message.Printer

// based on Wikipedia: / \ ? * : | " < >
// replace characters to create a valid directory name on all systems
var sanitizeFilenameReplacer *strings.Replacer = strings.NewReplacer("/", " ", "\\", "", "?", " ", "*", " ", ":", " ", "|", " ", "\"", " ", "<", " ", ">", " ")

var (
	methodFlag       string
	verboseFlag      bool
	versionFlag      bool
	quietFlag        bool
	outputFolderFlag string
	outputFormatFlag string
	thoroughFlag     bool
	braveFlag        bool
	debugFlag        bool
	num_workers      int
	preserveFlag     string
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

var nonAlphanumericRegex = regexp.MustCompile(`[^a-zA-Z0-9 ]+`)

func clearString(str string) string {
	return nonAlphanumericRegex.ReplaceAllString(strings.Trim(str, " "), "-")
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

func isNum(s string) bool {
	for _, v := range s {
		if v < '0' || v > '9' {
			return false
		}
	}
	return true
}

func processDataset(dataset dicom.Dataset, path string, oOrderPath string, in_file string) error {
	// in some special cases we want to skip this DICOM, e.g. DICOMDIR
	val, err := dataset.FindElementByTag(tag.MediaStorageSOPClassUID)
	if err == nil {
		var vs string = dicom.MustGetStrings(val.Value)[0]
		if vs == "1.2.840.10008.1.3.10" {
			// skip MediaStorageDirectoryStorage
			atomic.AddInt32(&counterError, 1)
			if debugFlag {
				fmt.Fprintf(os.Stderr, "[%d] ignore DICOMDIR file: \"%s\"\n\n", counterError, path)
			}
			return nil
		}
	}

	// go through all tags we need and pull those, use a map of tag.Tag as key and string as value
	// use together with dicomTags (tag.Tag as key and "{bla}" as value).
	dicomVals := make(map[tag.Tag]string, 0)
	for key := range dicomTags {
		val, err = dataset.FindElementByTag(key)
		if err == nil {
			var vs string = dicom.MustGetStrings(val.Value)[0]
			// this is used as a filename, we should sanitize them
			dicomVals[key] = sanitizeFilenameReplacer.Replace(vs)
			// we should check if vs is a safe string for a directory name
		} else {
			dicomVals[key] = ""
		}
	}

	//printMem()

	// if we filter we might not like this file
	var skipThisFile bool = false

	// now create the folder structure based on outputFolderFlag, treat the last entry as filename
	pps := outputFolderFlag
	for t := range dicomVals {
		// if we have a dicomTags[t] that contains an "==" we need to filter, only allow matching entries
		// TODO: could be done faster if we cache the regular expressions for each tag they are needed
		if strings.Contains(dicomTags[t], "==") {
			r := strings.Split(dicomTags[t], "==")[1]
			r = strings.TrimSuffix(r, "}")
			re := regexp.MustCompile(r)
			if !(re.MatchString(dicomVals[t])) {
				skipThisFile = true
				break
			}
		}

		if t == tag.SeriesNumber {
			sn, err := strconv.Atoi(dicomVals[tag.SeriesNumber])
			if err == nil {
				pps = strings.Replace(pps, "{SeriesNumber}", fmt.Sprintf("%03d", sn), -1)
			} else {
				pps = strings.Replace(pps, "{SeriesNumber}", dicomVals[tag.SeriesNumber], -1)
			}
		} else {
			pps = strings.Replace(pps, dicomTags[t], dicomVals[t], -1)
		}
	}
	if skipThisFile {
		atomic.AddInt32(&counterError, 1)
		if debugFlag {
			fmt.Fprintf(os.Stderr, "[%d] ignore file, cannot read as DICOM: \"%s\"\n\n", counterError, path)
		}
		return nil
	}

	// keep track of the patients, studies and series but only if we use verbose mode
	if !quietFlag {
		UpdateCounter(&listPatients, dicomVals[tag.PatientID])
		UpdateCounter(&listStudies, dicomVals[tag.StudyInstanceUID])
		UpdateCounter(&listSeries, dicomVals[tag.SeriesInstanceUID])
		// should be done with channels
		listStructuresChan <- []string{dicomVals[tag.PatientID], dicomVals[tag.StudyDate], dicomVals[tag.StudyInstanceUID], dicomVals[tag.SeriesInstanceUID], dicomVals[tag.Modality]}
	}

	pps = strings.Replace(pps, "{counter}", fmt.Sprintf("%06d", counter), -1) // use the global counter
	pps = strings.Replace(pps, " ", "-", -1)                                  // remove spaces

	pathPieces := splitPath(pps)
	piece := 0
	oOrderPatientPath := oOrderPath
	for piece < len(pathPieces)-1 {
		// this loop will concurrently try to create these folders, maybe they exist already even if we get an error in Mkdir
		oOrderPatientPath = filepath.Join(oOrderPatientPath, pathPieces[piece])
		if _, err := os.Stat(oOrderPatientPath); os.IsNotExist(err) {
			err := os.Mkdir(oOrderPatientPath, 0755)
			if err != nil {
				if _, err2 := os.Stat(oOrderPatientPath); os.IsNotExist(err2) {
					exitGracefully(fmt.Errorf("could not create data directory %s (%s)", oOrderPatientPath, err))
				}
			}
		}
		piece = piece + 1
	}
	// filename is
	fname := pathPieces[len(pathPieces)-1]

	outputPath := oOrderPatientPath

	outputPathFileName := fmt.Sprintf("%s/%s", outputPath, fname)
	_, err = os.Stat(outputPathFileName)
	var c int = 0
	atomic.AddInt32(&counter, 1)
	for !os.IsNotExist(err) {
		c = c + 1 // make filename unique by adding a number
		fname := fmt.Sprintf("%s_%03d%s", strings.TrimSuffix(fname, filepath.Ext(fname)), c, filepath.Ext(fname))
		outputPathFileName = fmt.Sprintf("%s/%s", outputPath, fname)
		//outputPathFileName := fmt.Sprintf("%s/%s_%03d.dcm", outputPath, SOPInstanceUID, c)
		_, err = os.Stat(outputPathFileName)
	}
	if verboseFlag && c != 0 {
		fmt.Fprintf(os.Stderr, "[%d] make file name unique: \"%s\"\n\n", counterError, outputPathFileName)
	}

	var bw int64 = 0
	err = nil
	if methodFlag == "copy" {
		bw, err = copyFileContents(in_file, outputPathFileName)
		// if we really copy the file we can also check for preserve
		_, ok := preserve["timestamp"]
		if ok {
			t, err := times.Stat(in_file)
			if err != nil {
				fmt.Fprintf(os.Stderr, "error, could not stat the output file")
			} else {
				os.Chtimes(outputPathFileName, t.AccessTime(), t.ModTime())
			}
		}
	} else if methodFlag == "link" {
		if err = os.Symlink(in_file, outputPathFileName); err != nil {
			fmt.Fprintf(os.Stderr, "Warning: could not create symlink %s for %s, %s\n", in_file, outputPathFileName, err)
		}
	} else if methodFlag == "emptyfile" { // TODO: do we keep this option?
		// don't do anything else
		emptyfile, e := os.Create(outputPathFileName)
		if e != nil {
			log.Fatal(e)
		}
		log.Println(emptyfile)
		emptyfile.Close()
	} else if methodFlag == "dirs_only" { // TODO: do we keep this option?
		// don't do anything else
		// remove the filename (keep the directory)
		opfn := filepath.Dir(outputPathFileName)
		os.MkdirAll(opfn, os.ModePerm)
	} else {
		// instead of copy we assume we want a symbolic link
		exitGracefully(fmt.Errorf("unknown option \"%s\" for method flag, we support only \"copy\" (default), \"link\" and \"dirs_only\"", methodFlag))
	}
	if err != nil {
		fmt.Println(err)
	}
	atomic.AddInt64(&bytesWritten, bw)
	return nil
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

	// print progress every second at least
	//spinner_c = int(math.Round(time.Since(startTime).Seconds()))
	//if !quietFlag && ((counter+counterError)%100 == 0) || (spinner_c != old_spinner_c) {
	//	numPatients := 0
	//	listPatients.Range(func(key, value interface{}) bool {
	//		numPatients = numPatients + 1
	//		return true
	//	})
	//	numStudies := 0
	//	listStudies.Range(func(key, value interface{}) bool {
	//		numStudies = numStudies + 1
	//		return true
	//	})
	//	numSeries := 0
	//	listSeries.Range(func(key, value interface{}) bool {
	//		numSeries = numSeries + 1
	//		return true
	//	})
	//	fmt_local.Printf("\033[A\033[2K\033[94;49m%s%d\033[37m [%.0f files / s] P %d S %d S %d [S %d]\033[39m\033[49m\n", spinner[(spinner_c)%len(spinner)], counter, (float64(counter))/time.Since(startTime).Seconds(), numPatients, numStudies, numSeries, counterError)
	//}
	//old_spinner_c = spinner_c

	// we can filter out files that take a long time if we allow only
	//  - files without an extension, or
	//  - files with .dcm as extension
	//  - files with an extension that contains only numbers (files named by UID)
	if !thoroughFlag && filepath.Ext(path) != "" {
		if strings.ToLower(filepath.Ext(path)) != ".dcm" && !isNum(filepath.Ext(path)[1:]) && len(filepath.Ext(path)) < 5 {
			atomic.AddInt32(&counterError, 1)
			if debugFlag {
				fmt.Fprintf(os.Stderr, "[%d] ignore file due to file extension: \"%s\"\n", counterError, path)
			}
			return nil // ignore this file
		}
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
		//} else if errors.Is(err, fs.ErrPermission) {
		//	exitGracefully(fmt.Errorf("could not create output data directory %s, %s", oOrderPath, err))
	} else if err != nil {
		exitGracefully(fmt.Errorf("could not create output data directory %s, %s", oOrderPath, err))
	}
	in_file := filepath.Join(InputDataPath, path)

	// Ok, we can try to be faster if we do not read the whole set, we would like
	// to also stop parsing after we have all the keys we need.
	// BenchmarkParser_NextAPI

	// Detect the filetype first
	if filepath.Ext(path) == ".tgz" {

	}

	dataset, err := dicom.ParseFile(in_file, nil, dicom.SkipPixelData()) // See also: dicom.Parse which has a generic io.Reader API.

	/*	TODO: switch to dicom.Parse as it can work with an io.Reader.
		    TODO: add the ability to detect tgz as file extension and get an io.Reader
			      from in-memory extraction of a tgz file. Check each DICOM file inside.
		    f, err := os.Open(filepath)
			if err != nil {
				return Dataset{}, err
			}
			defer f.Close()

			info, err := f.Stat()
			if err != nil {
				return Dataset{}, err
			}

			return Parse(f, info.Size(), frameChan, opts...) */

	//fmt.Printf("ParseFile time: %v %s\n", time.Since(sT), path)
	if err == nil { // switch in_file to an io.Reader? We could create a copy from inside a tgz... but links do not work anymore...
		if processDataset(dataset, path, oOrderPath, in_file) == nil {
			return nil
		}
	} else {
		atomic.AddInt32(&counterError, 1)
		if debugFlag {
			fmt.Fprintf(os.Stderr, "[%d] ignore file, cannot read as DICOM: \"%s\"\n\n", counterError, path)
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

func sort(source_paths []string, dest_path string) int32 {
	destination_path := dest_path

	if _, err := os.Stat(destination_path); os.IsNotExist(err) {
		err := os.Mkdir(destination_path, 0755)
		if err != nil {
			exitGracefully(fmt.Errorf("could not create output directory \"%s\", %s", destination_path, err.Error()))
		}
	}
	// storing information in global objects
	counter = 0 // we are using this to name DICOM files, not possible here!
	counterError = 0
	bytesWritten = 0
	ProcessDataPath = dest_path
	startTime = time.Now()
	if !quietFlag {
		fmt.Printf("\n")
	}
	for _, source_path := range source_paths {
		InputDataPath = source_path

		cwalk.NumWorkers = num_workers
		cwalk.BufferSize = cwalk.NumWorkers
		err := cwalk.WalkWithSymlinks(source_path, walkFunc)
		if err != nil {
			fmt.Printf("Error: (%s) %s\n", source_path, err.Error())
			/*for i, errors := range err.(cwalk.WalkerErrorList).ErrorList {
				fmt.Printf("Error [%d]: %s\n", i, errors)
			}*/
		}
	}
	if !quietFlag {
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

func translateStringOrFile(outputFolderFlag string) string {
	outputFolderFlag = strings.Trim(outputFolderFlag, " ") // remove any leading or trailing spaces
	if outputFolderFlag[0] == '@' {                        // should we read this as a filename?
		outputFolderFlag = outputFolderFlag[1:]
		if _, err := os.Stat(outputFolderFlag); errors.Is(err, os.ErrNotExist) {
			exitGracefully(fmt.Errorf("the path to folder file \"%s\" could not be found", outputFolderFlag))
		}
		b, err := os.ReadFile(outputFolderFlag) // just pass the file name
		if err != nil {
			exitGracefully(fmt.Errorf("file %s could not be read (%s)", outputFolderFlag, err))
		}
		outputFolderFlag = string(b)
		// remove lines that start with '#'
		s_list := strings.Split(strings.ReplaceAll(outputFolderFlag, "\r\n", "\n"), "\n")
		var new_list []string
		for _, ss := range s_list {
			ar := strings.Split(ss, "#")
			new_list = append(new_list, ar[0]) // remember the part before the comment character
		}
		outputFolderFlag = strings.Join(new_list, "")
	}
	re := regexp.MustCompile(`\r?\n`) // remove new lines
	outputFolderFlag = re.ReplaceAllString(outputFolderFlag, "")
	outputFolderFlag = strings.Replace(outputFolderFlag, "\t", "", -1) // do not allow tabs
	outputFolderFlag = strings.Replace(outputFolderFlag, " ", "", -1)  // do not allow spaces
	return outputFolderFlag
}

// -- string Value
type stringValue string

func newStringValue(val string, p *string) *stringValue {
	*p = val
	return (*stringValue)(p)
}

func (s *stringValue) Set(val string) error {
	*s = stringValue(val)
	return nil
}

func (s *stringValue) Get() any { return string(*s) }

func (s *stringValue) String() string { return string(*s) }

type Value interface {
	String() string
	Set(string) error
}

// isZeroValue determines whether the string represents the zero
// value for a flag.
func isZeroValue(flag *flag.Flag, value string) (ok bool, err error) {
	// Build a zero value of the flag's Value type, and see if the
	// result of calling its String method equals the value passed in.
	// This works unless the Value type is itself an interface type.
	typ := reflect.TypeOf(flag.Value)
	var z reflect.Value
	if typ.Kind() == reflect.Pointer {
		z = reflect.New(typ.Elem())
	} else {
		z = reflect.Zero(typ)
	}
	// Catch panics calling the String method, which shouldn't prevent the
	// usage message from being printed, but that we should report to the
	// user so that they know to fix their code.
	defer func() {
		if e := recover(); e != nil {
			if typ.Kind() == reflect.Pointer {
				typ = typ.Elem()
			}
			err = fmt.Errorf("panic calling String method on zero %v for flag %s: %v", typ, flag.Name, e)
		}
	}()
	return value == z.Interface().(Value).String(), nil
}

func MyPrintDefaults(f *flag.FlagSet) {
	var isZeroValueErrs []error
	f.VisitAll(func(flag *flag.Flag) {
		var b strings.Builder
		fmt.Fprintf(&b, "  -%s", flag.Name) // Two spaces before -; see next two comments.
		//name := flag.Name
		usage := flag.Usage
		//name, usage := flag.UnquoteUsage(flag)
		//if len(name) > 0 {
		//	b.WriteString(" ")
		//	b.WriteString(name)
		//}
		// Boolean flags of one ASCII letter are so common we
		// treat them specially, putting their usage on the same line.
		if b.Len() <= 4 { // space, space, '-', 'x'.
			b.WriteString("\t")
		} else {
			// Four spaces before the tab triggers good alignment
			// for both 4- and 8-space tab stops.
			b.WriteString("\n    \t")
		}
		b.WriteString(strings.ReplaceAll(usage, "\n", "\n    \t"))

		// Print the default value only if it differs to the zero value
		// for this flag type.
		if isZero, err := isZeroValue(flag, flag.DefValue); err != nil {
			isZeroValueErrs = append(isZeroValueErrs, err)
		} else if !isZero {
			if _, ok := flag.Value.(*stringValue); ok {
				// put quotes on the value
				fmt.Fprintf(&b, " (default %q)", flag.DefValue)
			} else {
				fmt.Fprintf(&b, " (default %v)", flag.DefValue)
			}
		}
		fmt.Fprint(f.Output(), b.String(), "\n")
	})
	// If calling String on any zero flag.Values triggered a panic, print
	// the messages after the full set of defaults so that the programmer
	// knows to fix the panic.
	if errs := isZeroValueErrs; len(errs) > 0 {
		fmt.Fprintln(f.Output())
		for _, err := range errs {
			fmt.Fprintln(f.Output(), err)
		}
	}
}

func main() {

	// Server for pprof
	//go func() {
	//	fmt.Println(http.ListenAndServe("localhost:6060", nil))
	//}()

	fmt_local = message.NewPrinter(message.MatchLanguage("en"))

	//rand.Seed(time.Now().UnixNano())
	// disable logging
	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "\n\033[1mNAME\033[0m\n\t%s - sort DICOM files into folders\n\n", os.Args[0])
		fmt.Fprintf(os.Stderr, "\033[1mUSAGE\033[0m\n\t%s (input folder) [(input folder N) ...] (output folder)\n", os.Args[0])
		fmt.Fprintf(os.Stderr, "\n\033[1mDESCRIPTION\033[0m\n\t\033[1msdcm\033[0m copies DICOM files from one directory to another. The output directory tree structure is user defined and based on DICOM meta-data.\n")
		fmt.Fprintf(os.Stderr, "\tAdditionally to named DICOM tags a numeric '{counter}' variable can be used. The argument to option 'folder' will be interpreted\n")
		fmt.Fprintf(os.Stderr, "\tas a filename if it starts with an '@'-character. The file may contain the folder path as text.\n\n")
		fmt.Fprintf(os.Stderr, "\t\t# Example format path file for sdcm\n")
		fmt.Fprintf(os.Stderr, "\t\t# Text after a '#' character is ignored. Spaces are also ignored.\n")
		fmt.Fprintf(os.Stderr, "\t\t# Uses empty strings if tags have no value or do not exist.\n")
		fmt.Fprintf(os.Stderr, "\t\t# Use this template with (save as default_format):\n")
		fmt.Fprintf(os.Stderr, "\t\t#     sdcm -format @default_format (input folder) (output folder)\n")
		fmt.Fprintf(os.Stderr, "\t\n")
		fmt.Fprintf(os.Stderr, "\t\t{PatientID}_{PatientName}/\n")
		fmt.Fprintf(os.Stderr, "\t\t	{StudyDate}_{StudyTime}/\n")
		fmt.Fprintf(os.Stderr, "\t\t		{SeriesNumber}_{SeriesDescription}/\n")
		fmt.Fprintf(os.Stderr, "\t\t			{Modality}_{SOPInstanceUID}.dcm\n")
		fmt.Fprintf(os.Stderr, "\n\tTo filter for specific DICOM files add a regular expression to the DICOM tag after '=='.\n")
		fmt.Fprintf(os.Stderr, "\n\tExample:\n")
		fmt.Fprintf(os.Stderr, "\t\t{Modality==(MR|CT)}\n")

		fmt.Fprintf(os.Stderr, "\n\033[1mOPTIONS\033[0m\n")
		// The defaults should not contain the type of a flag to work with 'compdef _gnu_generic sdcm'.
		MyPrintDefaults(flag.CommandLine)
		fmt.Fprintf(os.Stderr, "\n\033[1mENVIRONMENT\033[0m\n\tThe following environment variables affect the execution of sdcm:\n\n")
		fmt.Fprintf(os.Stderr, "\tSDCM_FOLDER_PATH\n\t\tThe default value for option -folder.\n\n")
	}

	log.SetFlags(0)
	log.SetOutput(io.Discard /*ioutil.Discard*/)

	flag.IntVar(&num_workers, "cpus", int(runtime.GOMAXPROCS(0)), "number of worker threads used for processing")
	flag.StringVar(&methodFlag, "method", "copy", "create either symbolic links (faster) or copy files. If dirs_only is used no files are created [copy|link|dirs_only]")
	defaultFolderFormat := "{PatientID}_{PatientName}/{StudyDate}_{StudyTime}/{SeriesNumber}_{SeriesDescription}/{Modality}_{SOPInstanceUID}.dcm"
	flag.StringVar(&outputFolderFlag, "folder", defaultFolderFormat, "specify the requested output folder path\n")
	flag.StringVar(&outputFormatFlag, "format", defaultFolderFormat, "same as -folder\n")
	flag.BoolVar(&verboseFlag, "verbose", false, "print more verbose output")
	flag.BoolVar(&quietFlag, "quiet", false, "do not print anything")
	flag.BoolVar(&thoroughFlag, "thorough", false, "do not filter files by extension, process all files (slower)")
	flag.BoolVar(&braveFlag, "brave", false, "write files even if the output folder already exists and it is not empty")
	flag.BoolVar(&debugFlag, "debug", false, "print verbose and add messages for skipped files")
	flag.BoolVar(&versionFlag, "version", false, "print the version number")
	flag.StringVar(&preserveFlag, "preserve", "", "preserves the timestamp if called with '-preserve timestamp'. This option only works together with '-method copy'")
	flag.Parse()

	if outputFormatFlag != "" {
		outputFolderFlag = outputFormatFlag
	}

	// allow output folder path to be specified by an environment variable
	if outputFolderFlag == "" {
		env_folder_path := os.Getenv("SDCM_FOLDER_PATH")
		if len(env_folder_path) > 0 {
			outputFolderFlag = env_folder_path
		}
	}
	// allow the outputFolderFlag to point to a file instead
	outputFolderFlag = translateStringOrFile(outputFolderFlag)

	// try to extract the tags requested in the outputFolderFlag
	dicomTags = make(map[tag.Tag]string, 0)
	re := regexp.MustCompile(`{[^{}]*}`)
	matches := re.FindAllString(outputFolderFlag, -1)
	for a := range matches {
		e := matches[a][1 : len(matches[a])-1]
		// we could be counter here, ignore and add later
		if e == "counter" {
			continue
		}
		// feature: if we find an == sign we use a regexp to filter, for now just extract the first part
		if strings.Contains(e, "==") {
			e = strings.Split(e, "==")[0]
		}

		// TODO: we should allow tags specified by group and element as well
		// e.g. {0010,0020} for PatientID
		if t, err := tag.FindByName(e); err == nil {
			dicomTags[t.Tag] = matches[a]
		} else {
			fmt.Fprintf(os.Stderr, "Warning, unknown DICOM tag with name \"%s\", cannot be used as a path variable, (%s)\n", matches[a], err)
		}
	}

	if debugFlag {
		quietFlag = false
	}

	if !quietFlag {
		// add three tags we need for book keeping
		dicomTags[tag.PatientID] = "{PatientID}"
		dicomTags[tag.StudyInstanceUID] = "{StudyInstanceUID}"
		dicomTags[tag.SeriesInstanceUID] = "{SeriesInstanceUID}"
	}

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

	// check preserveFlag
	preserve = make(map[string]bool, 0)
	if preserveFlag != "" {
		if methodFlag != "copy" {
			fmt.Println("warning: preserve is only supported for method 'copy'")
		}
		// allowed modes are timestamp
		pieces := strings.Split(preserveFlag, ",")
		for a := range pieces {
			//fmt.Printf("preserve request %s\n", pieces[a])
			if pieces[a] == "timestamp" {
				preserve[pieces[a]] = true
			} else {
				exitGracefully(fmt.Errorf("unknown preserve \"%s\"", pieces[a]))
			}
		}
	}

	//own_name = os.Args[0]

	if len(os.Args) < 3 {
		//flag.Usage()
		fmt.Fprintln(os.Stderr, "SDCM - sort DICOM files into folders\nUSAGE: sdcm (input folder) [(input folder N) ...] (output folder)")
		os.Exit(-1)
	}
	var input []string
	pos_args := flag.Args()
	for i := range pos_args[:len(pos_args)-1] {
		in, err := filepath.Abs(pos_args[i])
		if err != nil {
			exitGracefully(fmt.Errorf("input path \"%s\" could not be found", pos_args[i]))
		}
		input = append(input, in)
	}
	// we will error out of the output path already exists and is not empty
	if _, err := os.Stat(pos_args[len(pos_args)-1]); err == nil {
		isEmpty, _ := IsEmpty(pos_args[len(pos_args)-1])
		if !isEmpty && !braveFlag {
			exitGracefully(fmt.Errorf("output path %s already exists, cowardly refusing to continue. Clear its content, specify a new directory or be -brave", pos_args[len(pos_args)-1]))
		}
	}

	// check num_workers
	if num_workers < 1 {
		num_workers = 1
	}

	if !quietFlag {
		fmt.Printf("Parse %v...\n", input)
	}

	// print output every couple of milliseconds
	done := make(chan bool)
	if !quietFlag {
		ticker := time.NewTicker(250 * time.Millisecond)
		defer ticker.Stop()
		go func() {
			for {
				select {
				case <-ticker.C:
					// print progress every second at least
					spinner_c = int(math.Round(time.Since(startTime).Seconds()))
					if !quietFlag {
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
						fmt_local.Printf("\033[A\033[2K\033[94;49m%s%d\033[37m [%.0f files / s] P %d S %d S %d [S %d]\033[39m\033[49m\n", spinner[(spinner_c)%len(spinner)], counter, (float64(counter))/time.Since(startTime).Seconds(), numPatients, numStudies, numSeries, counterError)
					}
				case <-done:
					return
				}
			}
		}()
	}

	// use a channel listStructuresChan to store global information on the parsed DICOM files
	if !quietFlag {
		go func() {
			for entry := range listStructuresChan {
				SeriesInstanceUID := entry[3]
				listStructure[SeriesInstanceUID] = []string{entry[0], entry[1], entry[2], entry[4]}
			}
		}()
	}

	// all the work is done here
	numFiles := sort(input, pos_args[len(pos_args)-1])

	if !quietFlag {
		close(listStructuresChan) // close the channel to signal that we are done
		done <- true
	}

	if !quietFlag {
		s := "s"
		if numFiles == 1 {
			s = ""
		}
		fmt_local.Printf("✓ sorted %d file%s [%d non-DICOM files ignored or filtered]\n", numFiles, s, counterError)
	}
}
